package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const dayLayout = "2006-01-02"

var hexColor = regexp.MustCompile("^#[0-9A-Fa-f]{6}$")

func validate(r Record) error {
	if r == nil {
		return nil
	}
	if strings.TrimSpace(r["title"]) == "" {
		return errors.New("제목을 입력하세요.")
	}
	if r["kind"] == "group" {
		if !hexColor.MatchString(r["color"]) {
			return errors.New("그룹 색상을 확인하세요.")
		}
		return nil
	}
	start, e := time.Parse(dayLayout, r["start"])
	if e != nil {
		return errors.New("시작일을 확인하세요.")
	}
	switch r["kind"] {
	case "once":
	case "repeat":
		n, e := strconv.Atoi(r["interval"])
		if e != nil || n < 1 {
			return errors.New("반복 간격은 1일 이상이어야 합니다.")
		}
	case "span":
		end, e := time.Parse(dayLayout, r["end"])
		if e != nil || end.Before(start) {
			return errors.New("종료일은 시작일 이후여야 합니다.")
		}
		if r["deadline"] != "" {
			if _, e = time.Parse("15:04", r["deadline"]); e != nil {
				return e
			}
		}
	default:
		return errors.New("알 수 없는 일정 종류")
	}
	return nil
}
func occurs(r Record, d time.Time) bool {
	key := d.Format(dayLayout)
	if key < r["start"] || r["skip:"+key] == "yes" {
		return false
	}
	switch r["kind"] {
	case "once":
		return key == r["start"]
	case "span":
		return key <= r["end"]
	case "repeat":
		start, e := time.Parse(dayLayout, r["start"])
		n, err := strconv.Atoi(r["interval"])
		return e == nil && err == nil && n > 0 && (d.Unix()/86400-start.Unix()/86400)%int64(n) == 0
	}
	return false
}
func (a *App) Days(items map[string]Record, start string, count int) map[string][]string {
	days := map[string][]string{}
	d, e := time.Parse(dayLayout, start)
	if e != nil || count > 42 {
		return days
	}
	for i := 0; i < count; i++ {
		key := d.Format(dayLayout)
		for id, r := range items {
			if occurs(r, d) {
				days[key] = append(days[key], id)
			}
		}
		d = d.AddDate(0, 0, 1)
	}
	return days
}
