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

// This may be set each for each request if the http request includes a name
var UserName string

// These are the fields from the Chastikey API I care about
type Lock struct {
	LockID       int64  `json:"lockID"`
	LockName     string `json:"lockName"`
	LockedBy     string `json:"lockedBy"`
	LockFrozen   int64  `json:"lockFrozen"`
	FrozenByCard int64  `json:"lockFrozenByCard"`
	StartTime    int64  `json:"timestampLocked"`
	UnlockTime   int64  `json:"timestampUnlocked"`
	LastPicked   int64  `json:"timestampLastPicked"`
	NextPicked   int64  `json:"timestampNextPick"`
	Status       string `json:"status"`
	Combination  string `json:"combination"`
	// Card information
	CardHidden  int   `json:"cardInfoHidden"`
	DoubleCards int   `json:"doubleUpCards"`
	FreezeCards int   `json:"freezeCards"`
	GreenCards  int   `json:"greenCards"`
	GreenPicked int   `json:"greenCardsPicked"`
	RedCards    int   `json:"redCards"`
	ResetCards  int   `json:"resetCards"`
	YellowCards int   `json:"yellowCards"`
	Fixed       int   `json:"fixed"`
	Expected    int64 `json:"timestampExpectedUnlock"`
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

func format_time(val int64, name string) string {
	res := strconv.Itoa(int(val)) + " " + name
	if val != 1 {
		res += "s"
	}
	return res + " "
}

func time_to_days(val int64) string {
	var res string

	days := int64(val / 86400)
	if days > 0 {
		res += format_time(days, "day")
		val -= days * 86400
	}

	hours := int64(val / 3600)
	if hours > 0 {
		res += format_time(hours, "hour")
		val -= hours * 3600
	}

	mins := int64(val / 60)
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

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("DiscordID="+UserName)))

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

func one_lock(x int, y Lock) string {
	res := ""
	now := time.Now().Unix()
	dur := now - y.StartTime
	if y.UnlockTime != 0 {
		dur = y.UnlockTime - y.StartTime
	}
	pick := now - y.LastPicked
	if pick > 59 {
		pick = 60 * int64(pick/60)
	}
	next := y.NextPicked - now
	if next > 59 {
		next = 60 * int64(next/60)
	}

	res += "Lock " + strconv.Itoa(x)
	if y.LockName != "" {
		res += ", named " + y.LockName + ","
	}
	res += " is held by " + y.LockedBy + ", and "
	if y.UnlockTime != 0 {
		res += "ran"
	} else {
		res += "has been running"
	}
	res += " for " + time_to_days(dur) + ".  "
	if y.Combination == "" {
		if y.Fixed == 0 {
			res += "The last card was picked " + time_to_days(pick) + " ago.  "
			if y.LockFrozen == 0 {
				res += "The next card can be picked "
				if next <= 0 {
					res += "now"
				} else {
					res += "in " + time_to_days(next)
				}
				res += ".  "
			}
		} else {
			res += "This is a fixed lock.  "
		}

		if y.LockFrozen != 0 {
			res += "This lock is frozen by "
			if y.FrozenByCard != 0 {
				res += "card draw"
			} else {
				res += "key holder"
			}
			res += ".  "
		}
	} else {
		res += "This lock can be opened; the combination is " + y.Combination + ".  "
	}

	return res
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
		res += one_lock(x+1, y)
	}

	return res
}

func report_lock(id int, lock Lock) string {
	res := one_lock(id, lock)

	if lock.Combination == "" {
		if lock.CardHidden != 0 {
			res += "Card info is hidden"
		} else {
			if lock.Fixed == 0 {
				res += "You have picked " + strconv.Itoa(lock.GreenPicked) + " green card"
				if lock.GreenPicked != 1 {
					res += "s"
				}
				res += ".  There are " + strconv.Itoa(lock.GreenCards) + " green cards, "
				res += strconv.Itoa(lock.RedCards) + " red cards, "
				res += strconv.Itoa(lock.YellowCards) + " yellow cards, "
				res += strconv.Itoa(lock.FreezeCards) + " freeze cards, "
				res += strconv.Itoa(lock.DoubleCards) + " double cards, "
				res += "and " + strconv.Itoa(lock.ResetCards) + " reset cards remaining."
			} else {
				dur := lock.Expected - time.Now().Unix()
				if dur < 0 {
					res += "This lock is ready to unlock"
				} else {
					res += "This lock is expected to finish in " + time_to_days(dur)
				}
			}
		}
	}
	return res
}

func lock_summary(locks []Lock) string {
	res := "You have the following locks.  "
	for x, y := range locks {
		res += "Lock " + strconv.Itoa(x+1)
		if y.LockName != "" {
			res += ", named " + y.LockName + ","
		}
		res += " is held by " + y.LockedBy + ". "
	}
	return res
}

func list_locks() string {
	locks, errstr := talk_to_chastikey("lockeedata.php")
	if errstr != "" {
		return errstr
	}
	return lock_summary(locks)
}

// This is meant to search the locks by name, and return the matching
// lock.  However Amazon Alexa voice training means that things like "collar"
// get matched as "color" and this really doesn't work well.  So this code
// is currently unused.
func get_lock_by_name(arg []string) string {
	if len(arg) != 1 {
		return "You need to specify one number"
	}

	locks, errstr := talk_to_chastikey("lockeedata.php")
	if errstr != "" {
		return errstr
	}

	lockid := -1
	srch := strings.ToLower(arg[0])

	for x, y := range locks {
		if strings.ToLower(y.LockName) == srch {
			lockid = x
		}
	}

	if lockid == -1 {
		return "I could not find a lock with that name. " + lock_summary(locks)
	}

	return report_lock(lockid+1, locks[lockid])
}

func get_lock_by_id(arg []string) string {
	if len(arg) != 1 {
		return "You need to specify one number"
	}

	id, err := strconv.Atoi(arg[0])
	// Ideally we'd try to find this by name, but that doesn't work
	// (see comments on get_lock_by_name definition) so we just
	// return an error message
	if err != nil {
		// return get_lock_by_name(arg)
		return "The value you specified wasn't understandable as a number"
	}

	locks, errstr := talk_to_chastikey("lockeedata.php")
	if errstr != "" {
		return errstr
	}

	cnt := len(locks)
	if id < 1 || id > cnt {
		return "You need to pick a number between 1 and " + strconv.Itoa(cnt)
	}

	return report_lock(id, locks[id-1])
}

func get_help() string {
	return "Commands I understand are: " +
		"Ask chasty key for status.  " +
		"Ask chasty key about lock 1.  " +
		"Ask chasty key to list locks."
}

// Validate the command passed
func parse_command(cmd string, args []string) string {
	if cmd == "status" {
		return do_status()
	} else if cmd == "lockid" {
		return get_lock_by_id(args)
	} else if cmd == "listlocks" {
		return list_locks()
	} else if cmd == "AMAZON.HelpIntent" {
		return get_help()
	} else if strings.HasPrefix(cmd, "AMAZON") {
		return ""
	} else {
		return "I don't know how to " + cmd
	}
}

// Handle the Alexa query
func EchoIntentHandler(echoReq *alexa.EchoRequest, echoResp *alexa.EchoResponse) {
	var response string

	// This is a kludge to allow for multiple users to be used.
	// If the incoming request has a chastikey username in the userId
	// field then we'll use that.  Otherwise fall back to the username
	// in the config file
	// This only works in dev mode ("?_dev=1") because otherwise the
	// signature is broken.  dev mode also skips the application ID
	// check.
	// So this isn't really good, but it means other people can program
	// their alexa devices to call my hosted instance without needing
	// to run their own server.
	UserName = echoReq.GetUserID()
	if strings.HasPrefix(UserName, "amzn1.ask.account.") {
		UserName = configuration.UserName
	}

	log.Println("Using username " + UserName)

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

	// If no paramter is passed, default to "server"
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

	UserName = configuration.UserName

	if cmd == "server" {
		start_server(configuration.SkillID)
	} else {
		fmt.Println(parse_command(cmd, Args))
	}
}
