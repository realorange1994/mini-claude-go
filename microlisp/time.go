package microlisp

import (
	"fmt"
	"time"
)

func builtinGetInternalRunTime(args []*Value) (*Value, error) {
	return vnum(float64(time.Since(processStartTime).Milliseconds())), nil
}

func builtinEncodeUniversalTime(args []*Value) (*Value, error) {
	if len(args) < 6 {
		return nil, fmt.Errorf("encode-universal-time: need second minute hour day month year")
	}
	sec := int(toNum(primaryValue(args[0])))
	min := int(toNum(primaryValue(args[1])))
	hour := int(toNum(primaryValue(args[2])))
	day := int(toNum(primaryValue(args[3])))
	month := int(toNum(primaryValue(args[4])))
	year := int(toNum(primaryValue(args[5])))
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
	return vnum(float64(t.Unix() + 2208988800)), nil
}

func builtinDecodeUniversalTime(args []*Value) (*Value, error) {
	var ut float64
	if len(args) > 0 {
		ut = toNum(primaryValue(args[0]))
	} else {
		ut = float64(time.Now().Unix() + 2208988800)
	}
	t := time.Unix(int64(ut)-2208988800, 0)
	seconds := vnum(float64(t.Second()))
	minutes := vnum(float64(t.Minute()))
	hours := vnum(float64(t.Hour()))
	day := vnum(float64(t.Day()))
	month := vnum(float64(t.Month()))
	year := vnum(float64(t.Year()))
	dayOfWeek := vnum(float64(t.Weekday()))
	daylightSavingsP := vbool(false)
	timezone := vnum(0)
	return multiVal(seconds, minutes, hours, day, month, year, dayOfWeek, daylightSavingsP, timezone), nil
}

func builtinGetInternalRealTime(args []*Value) (*Value, error) {
	return vnum(float64(time.Now().UnixNano() / int64(time.Millisecond))), nil
}

func builtinGetUniversalTime(args []*Value) (*Value, error) {
	// Universal time: seconds since 1900-01-01 00:00:00 GMT
	t := time.Now()
	unixSec := t.Unix()
	// Offset from 1900 to 1970: 70 years worth of seconds
	offset := int64(2208988800)
	return vnum(float64(unixSec + offset)), nil
}
