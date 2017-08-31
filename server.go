package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/byuoitav/event-router-microservice/eventinfrastructure"
	"github.com/labstack/echo"
)

type toSend struct {
	Path  string
	Event string
}

var sendChannel chan toSend

func QueueEvent(context echo.Context) error {

	eventType := context.Param("type")
	eventCause := context.Param("cause")

	var event eventinfrastructure.Event
	err := context.Bind(&event)

	if err != nil || len(eventType) < 1 || len(eventCause) < 0 {
		return context.JSON(http.StatusBadRequest, fmt.Sprintf("Invalid Request"))
	}

	//Based on the Hostname of the event, parse out the building room
	//Salt path is /Type/Building/Room
	vals := strings.Split(event.Hostname, "-")

	saltPath := fmt.Sprintf("%s/%s/%s", event.Event.EventCause.String(), vals[0], vals[1])
	b, err := json.Marshal(event)

	//clean the value, escaping all single quotes
	eventToSend := fmt.Sprintf("%s", b)

	temp := toSend{Path: saltPath, Event: eventToSend}
	sendChannel <- temp

	return context.JSON(http.StatusOK, "Success")
}

func sendSaltEvent() {
	for {
		tmp := <-sendChannel
		log.Printf("Echoing event %v ", tmp.Event)

		cmd := exec.Command("salt-call", "event.send", tmp.Path, tmp.Event)

		err := cmd.Run()
		if err != nil {
			log.Printf("There was an error: %v", err.Error())
		}
	}
}

//a true means DHCP is turned on
func getDHCPStatus() (bool, error) {
	//we have to look at the contents of the /etc/dhcpcd.conf file to see if it's dhcp or not.

	file := "/etc/dhcpcd.conf"

	contents, err := ioutil.ReadFile(file)
	if err != nil {
		errString := fmt.Sprintf("There was an issue reading the dhcpcd file: %v", err.Error())
		log.Printf(errString)
		return false, errors.New(errString)
	}

	re, err := regexp.Compile(`(?m)^static ip_address`)
	if err != nil {
		return false, errors.New(fmt.Sprintf("There was an issue compiling the regex : %v", err.Error()))
	}

	matches := re.Match(contents)

	fmt.Printf("matches: %v", matches)

	return !matches, nil
}

func canToggle() bool {
	//we assume that when the dhcpcd.conf file was created it was copied and created a .other file - if there's no .other file, nothing we can do.
	file := "/etc/dhcpcd.conf"
	otherFile := "/etc/dhcpcd.conf.other"

	if _, err := os.Stat(otherFile); os.IsNotExist(err) {
		str := fmt.Sprintf("There isn't an other file, and we can't toggle DHCP")
		log.Printf(str)
		return false
	}
	if _, err := os.Stat(file); os.IsNotExist(err) {
		str := fmt.Sprintf("There isn't an file, and we can't toggle DHCP")
		log.Printf(str)
		return false
	}

	return true
}

func toggleDHCP() error {
	//we assume that when the dhcpcd.conf file was created it was copied and created a .other file - if there's no .other file, nothing we can do.
	file := "/etc/dhcpcd.conf"
	otherFile := "/etc/dhcpcd.conf.other"

	if !canToggle() {
		return errors.New("Can't toggle dhcp, files necessary aren't present")
	}

	//do the rename
	temp := "/etc/dhcpcd.conf.temp"
	err := os.Rename(file, temp)
	if err != nil {
		errStr := fmt.Sprintf("Couldn't rename file %v", err.Error())
		log.Printf("%v", errStr)
		return errors.New(errStr)
	}

	err = os.Rename(otherFile, file)
	if err != nil {
		errStr := fmt.Sprintf("Couldn't rename file %v", err.Error())
		log.Printf("%v", errStr)
		return errors.New(errStr)
	}

	err = os.Rename(temp, otherFile)
	if err != nil {
		errStr := fmt.Sprintf("Couldn't rename file %v", err.Error())
		log.Printf("%v", errStr)
		return errors.New(errStr)
	}

	_, err = exec.Command("sh", "-c", `sudo systemctl restart dhcpcd`).Output()
	if err != nil {
		errStr := fmt.Sprintf("Problem restarting the service: %v", err.Error())
		log.Printf(errStr)
		return errors.New(errStr)
	}

	return nil
}

func getStaticIP() string {
	//we can check if it's dhcp, if it is we get the .other file, look for the ip address there.
	return ""
}

func ToggleDHCP(context echo.Context) error {
	err := toggleDHCP()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, err.Error())
	}

	return GetDHCPState(context)
}

func GetDHCPState(context echo.Context) error {

	val, err := getDHCPStatus()
	if err != nil {
		return context.JSON(http.StatusInternalServerError, err.Error())
	}

	if val {
		return context.JSON(http.StatusOK, "dhcp")
	}
	return context.JSON(http.StatusOK, "static")

}

func reboot(context echo.Context) error {
	log.Printf("Rebooting")

	val, err := exec.Command("sh", "-c", `sudo reboot`).Output()

	log.Printf("%s", val)

	if err != nil {
		log.Printf("There was an error: %v", err.Error())
		return context.JSON(http.StatusInternalServerError, "Erorr: "+err.Error())
	}

	return context.JSON(http.StatusOK, "Rebooting")
}

func getDockerStatus(context echo.Context) error {
	log.Printf("Getting docker stuff")

	val, err := exec.Command("docker", "ps").Output()

	if err != nil {
		log.Printf("There was an error: %v", err.Error())
		return context.JSON(http.StatusInternalServerError, "Erorr: "+err.Error())
	}

	return context.String(http.StatusOK, string(val))
}

func main() {

	sendChannel = make(chan toSend, 1000)

	port := ":7010"
	router := echo.New()

	router.POST("/event/:type/:cause", QueueEvent)
	router.GET("/reboot", reboot)
	router.GET("/dockerStatus", getDockerStatus)
	router.GET("/dhcp", GetDHCPState)
	router.PUT("/dhcp", ToggleDHCP)

	server := http.Server{
		Addr:           port,
		MaxHeaderBytes: 1024 * 10,
	}

	go sendSaltEvent()

	router.StartServer(&server)
}
