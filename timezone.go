package main

func translateTZ(tzid string) string {
	switch tzid {
	case "W. Europe Standard Time":
		return "Europe/Berlin"
	case "India Standard Time":
		return "Asia/Kolkata"
	}
	return tzid
}
