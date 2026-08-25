package main

// translateMSTimezoneToIANA maps Microsoft Windows timezone names (as found in
// Exchange/Outlook CalDAV and ICS feeds, e.g. TZID="W. Europe Standard Time")
// to IANA timezone identifiers.
//
// Data source: Unicode CLDR windowsZones.xml (territory "001" primary mappings),
// cross-checked against
// https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/default-time-zones
// The Windows timezone ID set is legacy-stable (Microsoft keeps IDs compatible),
// so this table is effectively frozen and is intentionally vendored instead of
// pulled in as a dependency.
//
// Unknown identifiers are returned unchanged so callers can attempt
// time.LoadLocation on them directly (covers IANA names passed through).
func translateMSTimezoneToIANA(tzid string) string {
	switch tzid {
	case "Afghanistan Standard Time":
		return "Asia/Kabul"
	case "Alaskan Standard Time":
		return "America/Anchorage"
	case "Arab Standard Time":
		return "Asia/Riyadh"
	case "Arabian Standard Time":
		return "Asia/Dubai"
	case "Arabic Standard Time":
		return "Asia/Baghdad"
	case "Argentina Standard Time":
		return "America/Argentina/Buenos_Aires"
	case "Atlantic Standard Time":
		return "America/Halifax"
	case "AUS Central Standard Time":
		return "Australia/Darwin"
	case "AUS Eastern Standard Time":
		return "Australia/Sydney"
	case "Azerbaijan Standard Time":
		return "Asia/Baku"
	case "Azores Standard Time":
		return "Atlantic/Azores"
	case "Bangladesh Standard Time":
		return "Asia/Dhaka"
	case "Belarus Standard Time":
		return "Europe/Minsk"
	case "Canada Central Standard Time":
		return "America/Regina"
	case "Cape Verde Standard Time":
		return "Atlantic/Cape_Verde"
	case "Caucasus Standard Time":
		return "Asia/Yerevan"
	case "Cen. Australia Standard Time":
		return "Australia/Adelaide"
	case "Central America Standard Time":
		return "America/Guatemala"
	case "Central Asia Standard Time":
		return "Asia/Almaty"
	case "Central Europe Standard Time":
		return "Europe/Belgrade"
	case "Central European Standard Time":
		return "Europe/Warsaw"
	case "Central Pacific Standard Time":
		return "Pacific/Guadalcanal"
	// US Central Time — one of the most common Exchange zones
	case "Central Standard Time":
		return "America/Chicago"
	case "Central Standard Time (Mexico)":
		return "America/Mexico_City"
	case "China Standard Time":
		return "Asia/Shanghai"
	case "Dateline Standard Time":
		return "Etc/GMT+12"
	case "E. Africa Standard Time":
		return "Africa/Nairobi"
	case "E. Europe Standard Time":
		return "Europe/Bucharest"
	case "E. South America Standard Time":
		return "America/Sao_Paulo"
	case "Eastern Standard Time":
		return "America/New_York"
	case "Egypt Standard Time":
		return "Africa/Cairo"
	case "Fiji Standard Time":
		return "Pacific/Fiji"
	case "FLE Standard Time":
		return "Europe/Helsinki"
	case "Georgian Standard Time":
		return "Asia/Tbilisi"
	case "GMT Standard Time":
		return "Europe/London"
	case "Greenland Standard Time":
		return "America/Godthab"
	case "Greenwich Standard Time":
		return "Atlantic/Reykjavik"
	case "GTB Standard Time":
		return "Europe/Athens"
	case "Hawaiian Standard Time":
		return "Pacific/Honolulu"
	case "India Standard Time":
		return "Asia/Kolkata"
	case "Iran Standard Time":
		return "Asia/Tehran"
	case "Israel Standard Time":
		return "Asia/Jerusalem"
	case "Jordan Standard Time":
		return "Asia/Amman"
	case "Korea Standard Time":
		return "Asia/Seoul"
	case "Mauritius Standard Time":
		return "Indian/Mauritius"
	case "Middle East Standard Time":
		return "Asia/Beirut"
	case "Montevideo Standard Time":
		return "America/Montevideo"
	case "Morocco Standard Time":
		return "Africa/Casablanca"
	case "Mountain Standard Time":
		return "America/Denver"
	case "Myanmar Standard Time":
		return "Asia/Yangon"
	case "Nepal Standard Time":
		return "Asia/Kathmandu"
	case "Newfoundland Standard Time":
		return "America/St_Johns"
	case "New Zealand Standard Time":
		return "Pacific/Auckland"
	case "Pacific SA Standard Time":
		return "America/Santiago"
	case "Pacific Standard Time":
		return "America/Los_Angeles"
	case "Pakistan Standard Time":
		return "Asia/Karachi"
	case "Paraguay Standard Time":
		return "America/Asuncion"
	case "Romance Standard Time":
		return "Europe/Paris"
	case "Russian Standard Time":
		return "Europe/Moscow"
	case "SA Eastern Standard Time":
		return "America/Fortaleza"
	case "SA Pacific Standard Time":
		return "America/Bogota"
	case "SA Western Standard Time":
		return "America/Manaus"
	case "Samoa Standard Time":
		return "Pacific/Apia"
	case "SE Asia Standard Time":
		return "Asia/Bangkok"
	case "Singapore Standard Time":
		return "Asia/Singapore"
	case "South Africa Standard Time":
		return "Africa/Johannesburg"
	case "Sri Lanka Standard Time":
		return "Asia/Colombo"
	case "Syria Standard Time":
		return "Asia/Damascus"
	case "Taipei Standard Time":
		return "Asia/Taipei"
	case "Tokyo Standard Time":
		return "Asia/Tokyo"
	case "Tonga Standard Time":
		return "Pacific/Tongatapu"
	case "Türkiye Standard Time":
		return "Europe/Istanbul"
	case "Ulaanbaatar Standard Time":
		return "Asia/Ulaanbaatar"
	case "US Eastern Standard Time":
		return "America/Indiana/Indianapolis"
	case "US Mountain Standard Time":
		return "America/Phoenix"
	case "UTC":
		return "UTC"
	case "UTC-02":
		return "Etc/GMT+2"
	case "UTC-11":
		return "Etc/GMT+11"
	case "UTC+12":
		return "Etc/GMT-12"
	case "Venezuela Standard Time":
		return "America/Caracas"
	case "W. Central Africa Standard Time":
		return "Africa/Lagos"
	case "W. Europe Standard Time":
		return "Europe/Berlin"
	case "West Asia Standard Time":
		return "Asia/Tashkent"
	case "West Pacific Standard Time":
		return "Pacific/Port_Moresby"
	}
	return tzid
}
