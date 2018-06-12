package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/urfave/cli"
)

const (
	default_logfile = "./output.log"
)

var (
	log = logrus.New()
	app = cli.NewApp()
)

func init() {
	// Logging initialization
	initLogging()

	// Configuration initialization
	initViper()

	// App initialization
	initApp()
}

func initLogging() {
	log.Formatter = new(logrus.JSONFormatter)
	log.Formatter = new(logrus.TextFormatter)
	log.Formatter.(*logrus.TextFormatter).DisableTimestamp = true
	log.Level = logrus.InfoLevel
}

func initViper() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}
}

func initApp() {
	app.Name = "mstsc"
	app.Usage = "MotoJin Terminal Services Client"
	app.Version = "1.0"
}

func main() {

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "out",
			Value: "file",
			Usage: "log output mode (stdout, stderr)",
		},
		cli.StringFlag{
			Name:  "logfile",
			Value: default_logfile,
			Usage: "log output filename",
		},
		cli.StringFlag{
			Name:  "level",
			Value: "info",
			Usage: "log level (debug, warn, error, fatal, panic)",
		},
	}

	app.Action = func(c *cli.Context) error {
		// Change CLI config
		changeLogLevel(c.String("level"))
		changeLogOut(c.String("out"))

		file, err := os.OpenFile(c.String("logfile"), os.O_CREATE|os.O_WRONLY, 0666)
		if err == nil {
			log.Out = file
		} else {
			log.Fatalf("Failed to log to file, using default stderr, %s", err)
		}

		// Start loging
		log.Info("Start " + app.Name + " app")

		// Display input address UI and set remote connect address
		address := getHost(viper.Get("host"))

		// Display input login UI and set login infomation
		user, password := getUser(viper.Get("user"))

		// Display input password UI and set password when password is empty
		if len(password) == 0 {
			password = getPassword()
			log.WithFields(logrus.Fields{
				"password": password,
			}).Debug("Function getPassword")
		}

		// Excute remote connect commands
		execCommand("cmdkey /generic:TERMSRV/" + address + " /user:" + user + " /pass:" + password)
		time.Sleep(2 * time.Second)
		execCommand("start mstsc /f /v:" + address)
		time.Sleep(2 * time.Second)
		execCommand("cmdkey /delete:TERMSRV/" + address)

		// End logging
		log.Info("End " + app.Name + " app")

		return nil
	}

	// Run app
	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("CLI fatal error, %s", err)
	}
}

// Host connects to server
type Host struct {
	Name    string
	Type    string
	Address string
}

// Hosts represents connecting server list
type Hosts []Host

func getHost(hostList interface{}) (address string) {
	hostListSlice, ok := hostList.([]interface{})
	if !ok {
		log.WithFields(logrus.Fields{}).Error("Argument is not a slice")
		os.Exit(1)
	}

	var hosts Hosts
	for _, v := range hostListSlice {
		hosts = append(hosts, Host{
			Name:    v.(map[interface{}]interface{})["Name"].(string),
			Type:    v.(map[interface{}]interface{})["Type"].(string),
			Address: v.(map[interface{}]interface{})["Address"].(string),
		})
	}

	templates := &promptui.SelectTemplates{
		Label:    `{{ "?" | blue }} {{ . }} - DOWN:j UP:k`,
		Active:   `▸ {{ .Name | cyan | underline }} ({{ .Address | green}})`,
		Inactive: "  {{ .Name | cyan }} ({{ .Address | green }})",
		Selected: `{{ "✔" | green }} {{ .Name | bold }}`,
		Details: `
----------- Host -----------
{{ "Name:" | faint }}	{{ .Name }}
{{ "Type:" | faint }}	{{ .Type }}
{{ "Address:" | faint }}	{{ .Address }}`,
	}

	searcher := func(input string, index int) bool {
		host := hosts[index]
		name := strings.Replace(strings.ToLower(host.Name), " ", "", -1)
		input = strings.Replace(strings.ToLower(input), " ", "", -1)
		return strings.Contains(name, input)
	}

	prompt := promptui.Select{
		Label:     "Host",
		Items:     hosts,
		Templates: templates,
		Size:      4,
		Searcher:  searcher,
	}

	i, _, err := prompt.Run()
	if err != nil {
		log.Errorf("Prompt failed, %s", err)
		os.Exit(1)
	}

	address = hosts[i].Address
	log.WithFields(logrus.Fields{
		"address": address,
	}).Debug("Function getHost")

	return
}

func getUser(userList interface{}) (user string, password string) {
	userListSlice, ok := userList.([]interface{})
	if !ok {
		log.WithFields(logrus.Fields{}).Error("Argument is not a slice")
		os.Exit(1)
	}

	var users []string
	var login string
	for _, v := range userListSlice {
		if v.(map[interface{}]interface{})["Username"].(string) == "USERNAME" {
			login = v.(map[interface{}]interface{})["Domain"].(string) + "\\" + os.Getenv("USERNAME")
		} else {
			login = v.(map[interface{}]interface{})["Domain"].(string) + "\\" + v.(map[interface{}]interface{})["Username"].(string)
		}
		users = append(users, login)
	}

	prompt := promptui.SelectWithAdd{
		Label:    "User",
		Items:    users,
		AddLabel: "Other",
	}

	_, user, err := prompt.Run()
	if err != nil {
		log.Errorf("Prompt failed, %s", err)
		os.Exit(1)
	}

	for _, v := range userListSlice {
		if v.(map[interface{}]interface{})["Username"].(string) == "USERNAME" {
			login = v.(map[interface{}]interface{})["Domain"].(string) + "\\" + os.Getenv("USERNAME")
		} else {
			login = v.(map[interface{}]interface{})["Domain"].(string) + "\\" + v.(map[interface{}]interface{})["Username"].(string)
		}
		if login == user {
			password = v.(map[interface{}]interface{})["Password"].(string)
			if password == "NA" {
				password = ""
			}
		}
	}

	log.WithFields(logrus.Fields{
		"user":     user,
		"password": password,
	}).Debug("Function getUser")

	return
}

func getPassword() (password string) {
	validate := func(input string) error {
		if len(input) < 6 {
			return errors.New("Password must have more than 6 characters")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "Password",
		Validate: validate,
		Mask:     '*',
	}

	password, err := prompt.Run()

	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
		}).Error("Prompt failed")
		os.Exit(0)
	}

	return
}

func execCommand(command string) {
	log.WithFields(logrus.Fields{
		"command": command,
	}).Debug("Func execCommand")

	err := exec.Command("cmd", "/c", command).Run()
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
		}).Warn("Command Exec Error")
	}
}

func changeLogLevel(level string) {
	switch {
	case level == "debug":
		log.Level = logrus.DebugLevel
	case level == "warn":
		log.Level = logrus.WarnLevel
	case level == "error":
		log.Level = logrus.ErrorLevel
	case level == "fatal":
		log.Level = logrus.FatalLevel
	case level == "panic":
		log.Level = logrus.PanicLevel
	}
}

func changeLogOut(out string) {
	switch {
	case out == "stdout":
		log.Out = os.Stdout
	case out == "stderr":
		log.Out = os.Stderr
	}
}
