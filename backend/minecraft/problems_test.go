package minecraft

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDiagnoseCrash(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		exitErr   error
		wantTitle string
	}{
		{"wrong java", []string{"Exception in thread main java.lang.UnsupportedClassVersionError: x"}, errors.New("exit status 1"), "The server needs a newer Java"},
		{"port in use", []string{"**** FAILED TO BIND TO PORT!"}, errors.New("exit status 1"), "Port 25565 is already in use"},
		{"port in use alt", []string{"java.net.BindException: Address already in use"}, errors.New("exit status 1"), "Port 25565 is already in use"},
		{"heap too large", []string{"Error occurred during initialization of VM", "Could not reserve enough space for object heap"}, errors.New("exit status 1"), "Not enough memory to start"},
		{"out of memory", []string{"java.lang.OutOfMemoryError: Java heap space"}, errors.New("exit status 1"), "The server ran out of memory"},
		{"plugin failed", []string{"[ERROR] Error loading plugin Foo"}, errors.New("exit status 1"), "A plugin failed to load"},
		{"disk full", []string{"java.io.IOException: No space left on device"}, errors.New("exit status 1"), "The disk is full"},
		{"killed by system", []string{"[INFO] Done"}, errors.New("signal: killed"), "The system stopped the server"},
		{"unknown", []string{"[INFO] something"}, errors.New("exit status 1"), "The server stopped unexpectedly"},
		{"no logs no error", nil, nil, "The server stopped unexpectedly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := diagnoseCrash(tt.lines, tt.exitErr, 25565)
			if p == nil || p.Title != tt.wantTitle {
				t.Fatalf("title = %+v; want %q", p, tt.wantTitle)
			}
			if p.Hint == "" {
				t.Fatalf("problem has no hint: %+v", p)
			}
		})
	}
}

func TestDiagnoseCrashKeepsMatchingLineAsDetail(t *testing.T) {
	p := diagnoseCrash([]string{"a", "java.lang.OutOfMemoryError: Java heap space", "b"}, nil, 1)
	if !strings.Contains(p.Detail, "OutOfMemoryError") {
		t.Fatalf("detail = %q; want the matching log line", p.Detail)
	}
}

func TestFriendlyInstallError(t *testing.T) {
	tests := []struct{ raw, wantTitle string }{
		{`Java compatibility error: cannot determine Java requirement for type="fabric" version="27.1": unrecognized Minecraft version`, "Orexa cannot pick a Java version for this server"},
		{"Download failed: connection reset", "The server files could not be downloaded"},
		{"Version 9.9 is not available for paper", "That version is not available"},
		{"something odd", "The installation failed"},
	}
	for _, tt := range tests {
		p := friendlyInstallError(tt.raw)
		if p.Title != tt.wantTitle || p.Hint == "" || p.Detail == "" {
			t.Fatalf("friendlyInstallError(%q) = %+v; want title %q with hint and detail", tt.raw, p, tt.wantTitle)
		}
	}
}

func TestFriendlyJavaMessage(t *testing.T) {
	tests := []struct{ err, want string }{
		{"required Java 25 is not available (available: [17 21])", "This version needs Java 25, which is not included in this Orexa install."},
		{"no bundled JDK runtimes available", "No Java runtimes were found in this Orexa install."},
		{`cannot determine Java requirement for type="paper" version="x": unrecognized Minecraft version`, "Orexa does not know which Java this version needs yet."},
		{"other failure", "other failure"},
	}
	for _, tt := range tests {
		if got := friendlyJavaMessage(errors.New(tt.err)); got != tt.want {
			t.Fatalf("friendlyJavaMessage(%q) = %q; want %q", tt.err, got, tt.want)
		}
	}
}

func TestCrashRestartDecision(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	restart, kept := crashRestartDecision(nil, now)
	if !restart || len(kept) != 1 {
		t.Fatalf("first crash: restart=%v kept=%d; want true, 1", restart, len(kept))
	}

	var times []time.Time
	for i := 0; i < crashRestartLimit; i++ {
		restart, times = crashRestartDecision(times, now.Add(time.Duration(i)*time.Minute))
		if !restart {
			t.Fatalf("crash %d within the limit should restart", i+1)
		}
	}
	restart, times = crashRestartDecision(times, now.Add(4*time.Minute))
	if restart {
		t.Fatalf("crash %d in the window should not restart", crashRestartLimit+1)
	}

	restart, kept = crashRestartDecision(times, now.Add(crashRestartWindow+5*time.Minute))
	if !restart || len(kept) >= len(times) {
		t.Fatalf("after the window expires: restart=%v kept=%d; want true and old entries pruned", restart, len(kept))
	}
}

func TestLastLogLines(t *testing.T) {
	entries := []ConsoleLogEntry{{Seq: 1, Line: "a"}, {Seq: 2, Line: "b"}, {Seq: 3, Line: "c"}}
	got := lastLogLines(entries, 2)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("lastLogLines = %v; want [b c]", got)
	}
}

func TestFriendlyStartError(t *testing.T) {
	tests := []struct{ err, want string }{
		{"cannot start server due to Java compatibility: x", "Orexa cannot pick a Java version for this server. Try a different version."},
		{"no server jar found at /x/server.jar - upload the server .jar to the server folder", "The server file is missing. Open the File Browser and upload your server .jar, or reinstall the server."},
		{"server abc is already Running", "server abc is already Running"},
	}
	for _, tt := range tests {
		if got := FriendlyStartError(errors.New(tt.err)); got != tt.want {
			t.Fatalf("FriendlyStartError(%q) = %q; want %q", tt.err, got, tt.want)
		}
	}
}

func TestJavaPreflight(t *testing.T) {
	m := &Manager{javaResolver: testJavaResolver(17, 21, 25)}

	got := m.JavaPreflight("fabric", "26.1")
	if !got.OK || got.Required != 25 || got.Selected != 25 {
		t.Fatalf("fabric 26.1 = %+v; want ok with Java 25", got)
	}

	got = m.JavaPreflight("paper", "not-a-version")
	if got.OK || got.Message == "" {
		t.Fatalf("unknown version = %+v; want a plain message", got)
	}

	limited := &Manager{javaResolver: testJavaResolver(17, 21)}
	got = limited.JavaPreflight("paper", "26.1")
	if got.OK || got.Required != 25 || got.Message != "This version needs Java 25, which is not included in this Orexa install." {
		t.Fatalf("missing runtime = %+v; want a Java 25 message", got)
	}
}

func TestServerInfoPlayersUptimeAndProblem(t *testing.T) {
	started := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := &Manager{
		configs: map[string]*ServerConfig{
			"run":   {ID: "run", Name: "Run", Type: "Velocity", RestartOnCrash: true},
			"crash": {ID: "crash", Name: "Crash", Type: "Paper"},
			"fail":  {ID: "fail", Name: "Fail", Type: "Paper"},
		},
		running: map[string]*runningServer{
			"run":   {status: "Running", startedAt: started, players: map[string]*onlinePlayer{"a": {}, "b": {}}},
			"crash": {status: "Crashed", crashProblem: &Problem{Title: "The disk is full"}},
			"fail":  {status: "Error", installError: "Download failed: timeout"},
		},
	}

	run := m.serverInfo("run")
	if run.Players != 2 || run.StartedAt != "2026-01-01T12:00:00Z" || !run.RestartOnCrash || run.Problem != nil {
		t.Fatalf("running info = %+v; want 2 players, start time, restartOnCrash, no problem", run)
	}
	if crash := m.serverInfo("crash"); crash.Problem == nil || crash.Problem.Title != "The disk is full" || crash.StartedAt != "" {
		t.Fatalf("crashed info = %+v; want the crash problem and no start time", crash)
	}
	if fail := m.serverInfo("fail"); fail.Problem == nil || fail.Problem.Title != "The server files could not be downloaded" {
		t.Fatalf("install error info = %+v; want the friendly download problem", fail)
	}
}
