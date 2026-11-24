package main

import (
	"errors"
	"regexp"
	"sort"
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
func (a *App) Days(items map[string]Record, start string, count int) map[string][]string {
	days := map[string][]string{}
	viewStart, err := time.Parse(dayLayout, start)
	if err != nil || count <= 0 || count > 42 {
		return days
	}
	keys := make([]string, count)
	for i := range keys {
		keys[i] = viewStart.AddDate(0, 0, i).Format(dayLayout)
	}
	for id, r := range items {
		begin := r["start"]
		if begin > keys[count-1] {
			continue
		}
		switch r["kind"] {
		case "once":
			if begin >= start && r["skip:"+begin] != "yes" {
				days[begin] = append(days[begin], id)
			}
		case "span":
			if r["end"] < start {
				continue
			}
			for _, key := range keys[sort.SearchStrings(keys, begin):] {
				if key > r["end"] {
					break
				}
				if r["skip:"+key] != "yes" {
					days[key] = append(days[key], id)
				}
			}
		case "repeat":
			anchor, err := time.Parse(dayLayout, begin)
			interval, intervalErr := strconv.Atoi(r["interval"])
			if err != nil || intervalErr != nil || interval < 1 {
				continue
			}
			elapsed := (viewStart.Unix() - anchor.Unix()) / 86400
			for i, key := range keys {
				age := elapsed + int64(i)
				if age >= 0 && age%int64(interval) == 0 && r["skip:"+key] != "yes" {
					days[key] = append(days[key], id)
				}
			}
		}
	}
	return days
}
