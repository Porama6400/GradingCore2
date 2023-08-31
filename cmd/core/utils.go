package main

import "encoding/json"

func PrintJson(any interface{}) {
	bytes, err := json.MarshalIndent(any, "", "  ")
	if err != nil {
		return
	}
	println(string(bytes))
}
