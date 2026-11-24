package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Record map[string]string
type Change struct {
	ID            string
	Before, After Record
}
type Conflict struct {
	ID, Field     string
	Remote, Local string
}
type Resolution struct {
	Conflict Conflict
	UseLocal bool
}
type Snapshot struct {
	URL, Last, Message string
	Items              map[string]Record
	Pending            bool
	PendingCount       int
	NextSync           time.Time
	Conflicts          []Conflict
}
type Store struct{ root string }

var validID = regexp.MustCompile("^[a-zA-Z0-9-]{1,80}$")

const groupPrefix = "group:"

func recordPath(id string) string {
	if day, ok := strings.CutPrefix(id, "memo:"); ok {
		if _, err := time.Parse(dayLayout, day); err != nil {
			return ""
		}
		return "calender/memos/" + day + ".json"
	}
	stem, group := strings.CutPrefix(id, groupPrefix)
	if !validID.MatchString(stem) {
		return ""
	}
	if group {
		return "calender/groups/" + stem + ".json"
	}
	return "calender/events/" + stem + ".json"
}

func validateRecord(id string, r Record) error {
	if recordPath(id) == "" {
		return errors.New("잘못된 레코드 ID입니다.")
	}
	if strings.HasPrefix(id, "memo:") {
		if r != nil && (r["kind"] != "memo" || strings.TrimSpace(r["note"]) == "") {
			return errors.New("날짜 메모 내용을 확인하세요.")
		}
		return nil
	}
	if r != nil && (r["kind"] == "group") != strings.HasPrefix(id, groupPrefix) {
		return errors.New("레코드 종류가 경로와 일치하지 않습니다.")
	}
	return validate(r)
}

var githubURL = regexp.MustCompile("^https://github[.]com/[A-Za-z0-9_-]+/[A-Za-z0-9_.-]+/?$")

func (s Store) path(name string) string { return filepath.Join(s.root, name) }
func (s Store) locked(fn func() error) error {
	f, e := os.OpenFile(s.path("state.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	var ov windows.Overlapped
	if e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &ov); e != nil {
		return e
	}
	return fn()
}
func git(dir, index, input string, args ...string) (string, error) {
	data, err := gitOutput(dir, index, input, args...)
	return strings.TrimSpace(string(data)), err
}

func gitOutput(dir, index, input string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args = append([]string{"-c", "core.hooksPath=NUL", "-c", "user.name=calender", "-c", "user.email=calender@localhost"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
	}
	cmd.Stdin = strings.NewReader(input)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if e := cmd.Run(); e != nil {
		return nil, fmt.Errorf("Git: %s (%w)", strings.TrimSpace(errout.String()), e)
	}
	return out.Bytes(), nil
}
func (s Store) connect(url string) error {
	if !githubURL.MatchString(url) {
		return errors.New("GitHub HTTPS 저장소 주소를 입력하세요.")
	}
	if _, e := os.Stat(s.path("repo")); e == nil {
		return errors.New("이미 연결된 저장소가 있습니다.")
	}
	dir, e := os.MkdirTemp(s.root, "clone-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	if _, e = git(s.root, "", "", "clone", "--", url, dir); e != nil {
		return e
	}
	return os.Rename(dir, s.path("repo"))
}
func (s Store) pull() error {
	dir := s.path("repo")
	if _, e := git(dir, "", "", "rev-parse", "--verify", "HEAD"); e != nil {
		refs, e := git(dir, "", "", "ls-remote", "--heads", "origin")
		if e != nil {
			return e
		}
		if refs == "" {
			return nil
		}
	}
	_, e := git(dir, "", "", "pull", "--ff-only")
	return e
}
func (s Store) committedItems(previous string, cached map[string]Record) (map[string]Record, string, error) {
	head, err := git(s.path("repo"), "", "", "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return map[string]Record{}, "", nil // A newly connected, empty repository.
		}
		return nil, "", err
	}
	if head == previous && cached != nil {
		return cached, head, nil
	}
	items, err := s.itemsAt(head)
	return items, head, err
}

func (s Store) itemsAt(revision string) (map[string]Record, error) {
	dir := s.path("repo")
	items := map[string]Record{}
	paths, e := git(dir, "", "", "ls-tree", "--name-only", "-r", revision, "--", "calender/events", "calender/groups", "calender/memos")
	if e != nil {
		return nil, e
	}
	var ids []string
	var requests strings.Builder
	for _, p := range strings.Split(paths, "\n") {
		if p == "" {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		if strings.HasPrefix(p, "calender/groups/") {
			id = groupPrefix + id
		} else if strings.HasPrefix(p, "calender/memos/") {
			id = "memo:" + id
		}
		if p != recordPath(id) {
			continue
		}
		ids = append(ids, id)
		requests.WriteString(revision + ":" + p + "\n")
	}
	if len(ids) == 0 {
		return items, nil
	}
	// Batch output contains a header and an exact byte count for each blob.
	// Event JSON can contain newlines and multibyte titles.
	data, e := gitOutput(dir, "", requests.String(), "cat-file", "--batch")
	if e != nil {
		return nil, e
	}
	for _, id := range ids {
		header, body, ok := bytes.Cut(data, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("%s.json: 잘못된 Git 객체", id)
		}
		size, e := strconv.Atoi(fields[2])
		if e != nil || size < 0 || size >= len(body) || body[size] != '\n' {
			return nil, fmt.Errorf("%s.json: Git 데이터 길이 오류", id)
		}
		var r Record
		if e = json.Unmarshal(body[:size], &r); e != nil {
			return nil, fmt.Errorf("%s.json: %w", id, e)
		}
		if r == nil {
			return nil, fmt.Errorf("%s.json: 일정 내용이 없습니다", id)
		}
		if e = validateRecord(id, r); e != nil {
			return nil, fmt.Errorf("%s.json: %w", id, e)
		}
		items[id] = r
		data = body[size+1:]
	}
	return items, nil
}
func merge(c Change, remote Record) (Record, []Conflict) {
	if reflect.DeepEqual(c.Before, remote) || reflect.DeepEqual(c.After, remote) {
		return c.After, nil
	}
	if c.Before == nil || c.After == nil || remote == nil {
		l, _ := json.Marshal(c.After)
		r, _ := json.Marshal(remote)
		return remote, []Conflict{{c.ID, "*", string(r), string(l)}}
	}
	result := Record{}
	for k, v := range remote {
		result[k] = v
	}
	keys := map[string]bool{}
	for k := range c.Before {
		keys[k] = true
	}
	for k := range c.After {
		keys[k] = true
	}
	var conflicts []Conflict
	for k := range keys {
		before, after := c.Before[k], c.After[k]
		if before == after {
			continue
		}
		if remote[k] != before && remote[k] != after {
			conflicts = append(conflicts, Conflict{c.ID, k, remote[k], after})
			continue
		}
		if after == "" {
			delete(result, k)
		} else {
			result[k] = after
		}
	}
	if len(conflicts) == 0 && validateRecord(c.ID, result) != nil {
		l, _ := json.Marshal(c.After)
		r, _ := json.Marshal(remote)
		return remote, []Conflict{{c.ID, "*", string(r), string(l)}}
	}
	return result, conflicts
}

// A temporary index builds a candidate commit without modifying the pulled checkout.
// The durable journal can replay it on a newer parent after a rejected or ambiguous push.
func (s Store) publish(changes []Change, items map[string]Record) ([]Conflict, error) {
	dir := s.path("repo")
	indexDir, e := os.MkdirTemp(s.root, "index-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(indexDir)
	idx := filepath.Join(indexDir, "index")
	run := func(input string, args ...string) (string, error) { return git(dir, idx, input, args...) }
	head, _ := run("", "rev-parse", "--verify", "HEAD")
	if head != "" {
		if _, e = run("", "read-tree", head); e != nil {
			return nil, e
		}
	}
	var conflicts []Conflict
	modified := false
	for _, c := range changes {
		if recordPath(c.ID) == "" {
			return nil, errors.New("잘못된 일정 ID")
		}
		r, cs := merge(c, items[c.ID])
		conflicts = append(conflicts, cs...)
		if len(cs) > 0 || reflect.DeepEqual(r, items[c.ID]) {
			continue
		}
		if e = validateRecord(c.ID, r); e != nil {
			return nil, e
		}
		modified = true
		p := recordPath(c.ID)
		if r == nil {
			_, e = run("", "update-index", "--force-remove", "--", p)
		} else {
			data, _ := json.MarshalIndent(r, "", "  ")
			var blob string
			blob, e = run(string(data)+"\n", "hash-object", "-w", "--stdin")
			if e == nil {
				_, e = run("", "update-index", "--add", "--cacheinfo", "100644", blob, p)
			}
		}
		if e != nil {
			return nil, e
		}
	}
	if len(conflicts) > 0 || !modified {
		return conflicts, nil
	}
	tree, e := run("", "write-tree")
	if e != nil {
		return nil, e
	}
	args := []string{"commit-tree", tree, "-m", "Update calendar"}
	if head != "" {
		args = append(args, "-p", head)
	}
	commit, e := run("", args...)
	if e != nil {
		return nil, e
	}
	branch, e := run("", "symbolic-ref", "HEAD")
	if e != nil {
		return nil, e
	}
	if _, e = run("", "push", "origin", commit+":"+branch); e != nil {
		return nil, e
	}
	return nil, s.pull()
}
