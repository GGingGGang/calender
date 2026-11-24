package main

import (
	"context"
	"embed"
	"fmt"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	winopt "github.com/wailsapp/wails/v2/pkg/options/windows"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"

	"time"
	"unsafe"
)

//go:embed all:web
var assets embed.FS

type App struct {
	store    Store
	settings bool
	ctx      context.Context
	wake     chan struct{}
}

func (a *App) Mode() bool { return a.settings }
func (a *App) State(sync bool) (Snapshot, error) {
	if sync {
		return a.store.synchronize(false, time.Now(), nil)
	}
	return a.store.snapshot()
}
func (a *App) Version() (string, error) {
	info, e := os.Stat(a.store.path("batch.json"))
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size()), nil
}
func (a *App) Save(changes []Change) (Snapshot, error) {
	v, e := a.store.save(changes)
	if e == nil && v.PendingCount >= batchSize {
		select {
		case a.wake <- struct{}{}:
		default:
		}
	}
	return v, e
}
func (a *App) Resolve(fields []Conflict, local bool) (Snapshot, error) {
	choices := make([]Resolution, len(fields))
	for i, field := range fields {
		choices[i] = Resolution{Conflict: field, UseLocal: local}
	}
	return a.ResolveFields(choices)
}
func (a *App) ResolveFields(choices []Resolution) (Snapshot, error) {
	return a.store.synchronize(false, time.Now(), choices)
}
func (a *App) Connect(url string) error {
	return a.store.locked(func() error { return a.store.connect(url) })
}
func (a *App) Settings() error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	cmd := exec.Command(exe, "--settings")
	if e = cmd.Start(); e != nil {
		return e
	}
	return cmd.Process.Release()
}
func (a *App) Close() { wr.Quit(a.ctx) }

var user = windows.NewLazySystemDLL("user32.dll")

func call(name string, args ...uintptr) uintptr {
	r, _, _ := user.NewProc(name).Call(args...)
	return r
}
func str(s string) uintptr { p, _ := windows.UTF16PtrFromString(s); return uintptr(unsafe.Pointer(p)) }
func desktop() uintptr {
	progman := call("FindWindowW", str("Progman"), 0)
	if view := call("FindWindowExW", progman, 0, str("SHELLDLL_DefView"), 0); progman != 0 && view != 0 {
		return view
	}
	var worker uintptr
	for {
		worker = call("FindWindowExW", 0, worker, str("WorkerW"), 0)
		if worker == 0 {
			return 0
		}
		if view := call("FindWindowExW", worker, 0, str("SHELLDLL_DefView"), 0); view != 0 {
			return view
		}
	}
}

type bounds struct{ X, Y, W, H int }

func attachToDesktop(hwnd uintptr) uintptr {
	parent := desktop()
	const (
		gwlStyle  = ^uintptr(15)
		wsCaption = 0x00c00000
		wsSysMenu = 0x00080000
		wsMinBox  = 0x00020000
		wsMaxBox  = 0x00010000
		wsPopup   = 0x80000000
		wsChild   = 0x40000000
	)
	// Wails starts with WS_OVERLAPPEDWINDOW even in frameless mode.
	// Drop its caption and controls, keeping WS_THICKFRAME for edge resizing.
	original := call("GetWindowLongW", hwnd, gwlStyle)
	style := original &^ (wsCaption | wsSysMenu | wsMinBox | wsMaxBox)
	if parent != 0 {
		call("SetWindowLongW", hwnd, gwlStyle, style&^wsPopup|wsChild)
		call("SetParent", hwnd, parent)
		if call("GetParent", hwnd) != parent {
			parent = 0
		}
	}
	if parent == 0 {
		if call("GetParent", hwnd) == 0 {
			style = style&^wsChild | wsPopup
		}
		if call("GetWindowLongW", hwnd, gwlStyle) == style {
			return 0
		}
		call("SetWindowLongW", hwnd, gwlStyle, style)
	}
	// Recalculate the frame without moving, resizing, activating or showing the window.
	const frameFlags = 0x20 | 0x10 | 0x04 | 0x02 | 0x01
	call("SetWindowPos", hwnd, 0, 0, 0, 0, 0, frameFlags)
	return parent
}

func (a *App) ready(ctx context.Context) {
	go func() {
		for {
			state, _ := a.store.synchronize(true, time.Now(), nil)
			delay := time.Until(state.NextSync)
			if delay <= 0 || delay > syncInterval {
				delay = syncInterval
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-a.wake:
				timer.Stop()
			case <-timer.C:
			}
		}
	}()
	if a.settings {
		return
	}
	hwnd := call("FindWindowW", str("CalenderDesktopWindow"), 0)
	if hwnd == 0 {
		wr.WindowShow(ctx)
		return
	}
	file := a.store.path("position.json")
	var saved bounds
	if readJSON(file, &saved) == nil && saved.W >= 540 && saved.H >= 330 {
		wr.WindowSetSize(ctx, saved.W, saved.H)
		wr.WindowSetPosition(ctx, saved.X, saved.Y)
	}
	parent := attachToDesktop(hwnd)
	wr.WindowShow(ctx)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if parent == 0 || call("GetParent", hwnd) != parent || call("IsWindow", parent) == 0 {
					parent = attachToDesktop(hwnd)
				}
				x, y := wr.WindowGetPosition(ctx)
				w, h := wr.WindowGetSize(ctx)
				now := bounds{x, y, w, h}
				if now != saved && w >= 540 && h >= 330 {
					if atomicJSON(file, now) == nil {
						saved = now
					}
				}
			}
		}
	}()
}
func main() {
	root, e := os.UserConfigDir()
	if e != nil {
		panic(e)
	}
	root = filepath.Join(root, "calender")
	if e = os.MkdirAll(root, 0700); e != nil {
		panic(e)
	}
	settings := len(os.Args) > 1 && os.Args[1] == "--settings"
	a := &App{store: Store{root}, settings: settings, wake: make(chan struct{}, 1)}
	title := "calender"
	class := "CalenderDesktopWindow"
	if settings {
		title += " · 설정"
		class = "CalenderSettingsWindow"
	}
	mutexName, _ := windows.UTF16PtrFromString("Local\\" + class)
	mutex, e := windows.CreateMutex(nil, false, mutexName)
	if e == windows.ERROR_ALREADY_EXISTS {
		w := call("FindWindowW", str(class), 0)
		if w != 0 {
			call("SetForegroundWindow", w)
		}
		return
	}
	if e != nil {
		panic(e)
	}
	defer windows.CloseHandle(mutex)
	e = wails.Run(&options.App{Title: title, Width: 850, Height: 540, MinWidth: 580, MinHeight: 360, Frameless: !settings, StartHidden: !settings, BackgroundColour: options.NewRGBA(0, 0, 0, 0), AssetServer: &assetserver.Options{Assets: assets}, OnStartup: func(ctx context.Context) { a.ctx = ctx }, OnDomReady: a.ready, Bind: []interface{}{a},
		Windows: &winopt.Options{
			Theme:                             winopt.Dark,
			DisableFramelessWindowDecorations: !settings,
			WebviewIsTransparent:              !settings,
			WindowIsTranslucent:               !settings,
			WindowClassName:                   class,
			WebviewUserDataPath:               a.store.path("webview-" + class),
		},
	})
	if e != nil {
		call("MessageBoxW", 0, str(fmt.Sprint(e)), str(title), 16)
	}
}
