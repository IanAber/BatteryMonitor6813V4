package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
)

type errorObj struct {
	Device string
	Err    string
}

type JSONError struct {
	Success bool        `json:"success"`
	Errors  []*errorObj `json:"errors"`
}

func (j *JSONError) AddErrorString(device string, err string) error {
	e := new(errorObj)
	e.Device = device
	e.Err = err
	j.Errors = append(j.Errors, e)
	return fmt.Errorf("Device : %s | error %s", device, err)
}

func (j *JSONError) AddError(device string, err error) error {
	e := new(errorObj)
	e.Device = device
	e.Err = err.Error()
	j.Errors = append(j.Errors, e)
	return err
}

func (j *JSONError) String() string {
	if s, err := json.Marshal(j); err != nil {
		log.Print(err)
		return ""
	} else {
		return string(s)
	}
}

func (j *JSONError) ReturnError(w http.ResponseWriter, retCode int) {
	// Set the returned type to application/json
	w.Header().Set("Content-Type", "application/json")
	// Set the retCode
	w.WriteHeader(retCode)
	// Return the JSON content
	_, err := fmt.Fprint(w, j.String())
	if err != nil {
		log.Println(err)
	}
}

func ReturnJSONError(w http.ResponseWriter, device string, err error, httpReturnCode int, bLog bool) {
	var jErr JSONError

	_ = jErr.AddError(device, err)
	jErr.Success = false
	jErr.ReturnError(w, httpReturnCode)
	if bLog {
		_, caller, line, _ := runtime.Caller(1)
		log.Printf("%s : %d : %v", caller, line, err)
	}
}

func ReturnJSONErrorString(w http.ResponseWriter, device string, errStr string, httpReturnCode int, bLog bool) {
	var jErr JSONError

	err := jErr.AddErrorString(device, errStr)
	jErr.Success = false
	jErr.ReturnError(w, httpReturnCode)
	if bLog {
		log.Print(err)
	}
}
