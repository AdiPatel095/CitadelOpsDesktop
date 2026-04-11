package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

func main() {
	files := []string{"../gbd.json", "../Temp3.json", "../Temp5.json"}
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		// Search recursively for "gpa"
		var findGPA func(interface{}) bool
		findGPA = func(node interface{}) bool {
			if obj, ok := node.(map[string]interface{}); ok {
				if gpa, has := obj["gpa"]; has {
					if gpaObj, isMap := gpa.(map[string]interface{}); isMap {
						fmt.Printf("Found gpa keys in %s:\n", file)
						for k, v := range gpaObj {
							fmt.Printf("%s: %v\n", k, v)
						}
						return true
					}
				}
				for _, v := range obj {
					if findGPA(v) {
						return true
					}
				}
			} else if arr, ok := node.([]interface{}); ok {
				for _, v := range arr {
					if findGPA(v) {
						return true
					}
				}
			}
			return false
		}

		findGPA(m)
	}
}
