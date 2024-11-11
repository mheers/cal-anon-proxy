package main

import (
	_ "time/tzdata"
)

func translateTZ(tzid string) string {
	switch tzid {
	case "W. Europe Standard Time":
		return "Europe/London"
	case "India Standard Time":
		return "Asia/Kolkata"
	}
	return tzid
}