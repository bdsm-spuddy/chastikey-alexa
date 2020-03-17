package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	alexa "github.com/mikeflynn/go-alexa/skillserver"
	"github.com/tkanos/gonfig"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Information we read from the config file
type Configuration struct {
	SkillID   string
	ApiID     string
	ApiSecret string
	UserName  string
}

// Global variables based on that config
var configuration Configuration

// These are the fields from the Chastikey API I care about
type Lock struct {
	LockID      int64  `json:"lockID"`
	LockName    string `json:"lockName"`
	LockedBy    string `json:"lockedBy"`
	LockFrozen  int64  `json:"lockFrozen"`
	StartTime   int64  `json:"timestampLocked"`
	LastPicked  int64  `json:"timestampLastPicked"`
	Status      string `json:"status"`
	Combination string `json:"combination"`
}

type Chastikey struct {
	Locks []Lock `json:"locks"`
}

// Where do config files live?
func UserHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home + "\\"
	}
	return os.Getenv("HOME") + "/"
}

func format_time(val int, name string) string {
	res := strconv.Itoa(val) + " " + name
	if val != 1 {
		res += "s"
	}
	return res + " "
}

func time_to_days(val int) string {
	var res string

	days := int(val / 86400)
	if days > 0 {
		res += format_time(days, "day")
		val -= days * 86400
	}

	hours := int(val / 3600)
	if hours > 0 {
		res += format_time(hours, "hour")
		val -= hours * 3600
	}

	mins := int(val / 60)
	if mins > 0 {
		res += format_time(mins, "minute")
		val -= mins * 60
	}

	if val > 0 {
		res += format_time(val, "second")
	}

	return strings.TrimSpace(res)
}

func do_talk_to_chastikey(cmd string) (string, string) {
	if os.Getenv("DEBUG") != "" {
		return os.Getenv("DEBUG"), ""
	}

	url := "https://api.chastikey.com/v0.5/" + cmd

	fmt.Println("Calling " + cmd)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("Username="+configuration.UserName)))

	if err != nil {
		return "", "Problems setting up the API: " + err.Error()
	}

	req.Header.Set("ClientID", configuration.ApiID)
	req.Header.Set("ClientSecret", configuration.ApiSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return "", "Problems calling the API: " + err.Error()
	}

	// What a hassle, just to get the string
	// http://dlintw.github.io/gobyexample/public/http-client.html
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err.Error()
	}
	res := string(body)

	if resp.StatusCode != 200 {
		fmt.Println("Bad result: " + res)
		return "", "Error from API: " + resp.Status
	}
	return res, ""
}

func talk_to_chastikey(cmd string) ([]Lock, string) {
	var chastikey Chastikey

	json_str, http_err := do_talk_to_chastikey(cmd)
	if http_err != "" {
		return nil, http_err
	}

	err := json.Unmarshal([]byte(json_str), &chastikey)
	if err != nil {
		return nil, "Could not understand API results: " + err.Error()
	}

	// We want to look at the Locks
	locks := chastikey.Locks

	// Let's make sure the locks are in LockID order.  They
	// probably are, anyway, but let's not make assumptions :-)
	// This will try to maintain consistency across calls.
	sort.Slice(locks, func(i, j int) bool {
		return locks[i].LockID < locks[j].LockID
	})

	return locks, ""
}

// Ask Chastikey for the user status and generate a friendly response
func do_status() string {
	locks, err := talk_to_chastikey("lockeedata.php")
	if err != "" {
		return err
	}

	cnt := len(locks)
	res := "You have " + strconv.Itoa(cnt) + " lock"
	if cnt != 1 {
		res += "s"
	}
	res += ".  "
	for x, y := range locks {
		dur := time.Now().Unix() - y.StartTime
		pick := time.Now().Unix() - y.LastPicked
		res += "Lock " + strconv.Itoa(x+1)
		if (y.LockName != "" ) {
			res += ", named " + y.LockName + ","
		}
		res += " is held by " + y.LockedBy + ", "
		res += "and has been running for " + time_to_days(int(dur)) + ".  "
		res += "The last card was picked " + time_to_days(60*int(pick/60)) + " ago.  "
		if y.LockFrozen != 0 {
			res += "This lock is frozen.  "
		}
	}

	return res
}

// Validate the command passed
// We should only ever have one parameter, so pass that when needed
func parse_command(cmd string, args []string) string {
	if cmd == "status" {
		return do_status()
	} else {
		return "I don't know how to " + cmd
	}
}

// Handle the Alexa query
func EchoIntentHandler(echoReq *alexa.EchoRequest, echoResp *alexa.EchoResponse) {
	var response string

	fn := echoReq.GetIntentName()

	log.Println("Got Alexa event " + fn)

	// We need to build an argument array.	This can be simple; we
	// only expect to have a single arguement, so lets iterate over
	// the slots and just add them
	var args []string

	for slot, _ := range echoReq.AllSlots() {
		value, _ := echoReq.GetSlotValue(slot)
		// log.Printf("  with slot = %v\n", slot)
		// log.Printf("	  value = %v\n", value)
		args = append(args, value)
	}

	response = parse_command(fn, args)
	log.Println("Got response " + response)
	echoResp.OutputSpeech(response)
}

// Start the Alexa server
func start_server(amazon_skill_id string) {
	Applications := map[string]interface{}{
		"/echo/chastikey": alexa.EchoApplication{ // Route
			AppID:    amazon_skill_id,
			OnIntent: EchoIntentHandler,
		},
	}

	alexa.Run(Applications, "3001")
	fmt.Println("Server started")
}

func main() {
	Args := os.Args

	// If no paramter is passed, default to "identify"
	// otherwise split command line args
	var cmd string = "server"
	if len(Args) > 1 {
		cmd = Args[1]
		Args = Args[2:]
	}

	// Try and find the config file
	config_file := UserHomeDir() + ".chastikey"
	fmt.Println("Using configuration file " + config_file)

	parse := gonfig.GetConf(config_file, &configuration)
	if parse != nil {
		fmt.Println("Error parsing "+config_file+"\n", parse)
		os.Exit(-1)
	}

	if configuration.SkillID == "" {
		fmt.Println("Skill ID is not defined.  Aborted")
		os.Exit(255)
	}

	if configuration.UserName == "" {
		fmt.Println("Chastikey UserName is not defined.	Aborted")
		os.Exit(255)
	}

	if cmd == "server" {
		start_server(configuration.SkillID)
	} else {
		fmt.Println(parse_command(cmd, Args))
	}
}
