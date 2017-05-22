package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
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

	server := http.Server{
		Addr:           port,
		MaxHeaderBytes: 1024 * 10,
	}

	go sendSaltEvent()

	router.StartServer(&server)
}
