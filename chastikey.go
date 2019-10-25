package main

import (
	"encoding/json"
	"fmt"
	alexa "github.com/mikeflynn/go-alexa/skillserver"
	"github.com/tkanos/gonfig"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"strconv"
	"time"
)

// Information we read from the config file
type Configuration struct {
	SkillID string
	UserID  string
}

// Global variables based on that config
var configuration Configuration
var UserID string

// These are the fields from the Chastikey API I care about
type Lock struct {
	LockID   int64 `json:"lockID"`
	LockedBy string `json:"lockedBy"`
        LockFrozen int64 `json:"lockFrozen"`
	StartTime int64 `json:"timestampLocked"`
	Status string `json:"status"`
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

	res += format_time(val, "second")

	return strings.TrimSpace(res)
}

func parse_api(json_str string) string {
	var chastikey Chastikey
	err := json.Unmarshal([]byte(json_str), &chastikey)
	if err != nil {
		return "Could not understand API results: "+err.Error()
	}

	// We want to look at the chastity session
	s := chastikey.Locks

        cnt := len(s)
	res := "You have " + strconv.Itoa(cnt) + " lock"
        if cnt != 1 {
		res += "s"
	}
	res += ".  "
	for x,y := range s {
		dur := time.Now().Unix()-y.StartTime
		res += "Lock " + strconv.Itoa(x+1) + " is held by " + y.LockedBy + ", and has been running for " + time_to_days(int(dur)) + ".  "
		if (y.LockFrozen != 0) {
			res += "This lock is frozen.  "
		}
	}

	return res
}

func talk_to_chastikey(cmd string) (string, string) {
	if os.Getenv("DEBUG") != "" {
		return os.Getenv("DEBUG"),""
	}

	url := "https://api.chastikey.com/v0.2/" + cmd + "?userID=" + UserID

	fmt.Println("Calling " + cmd)
	resp, err := http.Get(url)

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

// Ask Chastikey for the user status and generate a friendly response
func do_status() string {
	s, err := talk_to_chastikey("listlocks.php")
	if err != "" {
		return err
	}

	return parse_api(s)
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
		fmt.Println("Error parsing " + config_file + "\n",parse)
		os.Exit(-1)
	}

	amazon_skill_id := configuration.SkillID
	UserID = configuration.UserID

	if amazon_skill_id == "" {
		fmt.Println("Skill ID is not defined.  Aborted")
		os.Exit(255)
	}

	if UserID == "" {
		fmt.Println("Chastikey UserID is not defined.	Aborted")
		os.Exit(255)
	}

	if cmd == "server" {
		start_server(amazon_skill_id)
	} else {
		fmt.Println(parse_command(cmd, Args))
	}
}
