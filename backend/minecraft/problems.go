package minecraft

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Problem struct {
	Title  string `json:"title"`
	Hint   string `json:"hint"`
	Detail string `json:"detail,omitempty"`
}

const (
	crashRestartLimit  = 3
	crashRestartWindow = 10 * time.Minute
	crashRestartDelay  = 5 * time.Second
	crashLogLines      = 200
	problemDetailMax   = 300
)

var javaAvailabilityPattern = regexp.MustCompile(`required Java (\d+) is not available`)

func friendlyJavaMessage(err error) string {
	if err == nil {
		return ""
	}
	raw := err.Error()
	if m := javaAvailabilityPattern.FindStringSubmatch(raw); m != nil {
		return fmt.Sprintf("This version needs Java %s, which is not included in this Orexa install.", m[1])
	}
	switch {
	case strings.Contains(raw, "no bundled JDK runtimes available"):
		return "No Java runtimes were found in this Orexa install."
	case strings.Contains(raw, "cannot determine Java requirement"):
		return "Orexa does not know which Java this version needs yet."
	}
	return raw
}

func friendlyInstallError(raw string) *Problem {
	detail := truncateDetail(raw)
	switch {
	case strings.Contains(raw, "Java compatibility error"):
		return &Problem{
			Title:  "Orexa cannot pick a Java version for this server",
			Hint:   "Try a different version, or update Orexa if this one is very new.",
			Detail: detail,
		}
	case strings.Contains(raw, "Download failed"):
		return &Problem{
			Title:  "The server files could not be downloaded",
			Hint:   "Check your internet connection, then press Retry.",
			Detail: detail,
		}
	case strings.Contains(raw, "is not available for"):
		return &Problem{
			Title:  "That version is not available",
			Hint:   "Pick a different version and try again.",
			Detail: detail,
		}
	}
	return &Problem{
		Title:  "The installation failed",
		Hint:   "Press Retry. If it keeps failing, open the Logs tab for details.",
		Detail: detail,
	}
}

func FriendlyStartError(err error) string {
	raw := err.Error()
	switch {
	case strings.Contains(raw, "Java compatibility"):
		return "Orexa cannot pick a Java version for this server. Try a different version."
	case strings.Contains(raw, "no server jar found"):
		return "The server file is missing. Open the File Browser and upload your server .jar, or reinstall the server."
	}
	return raw
}

type crashSignature struct {
	needles []string
	problem func(port int) Problem
}

var crashSignatures = []crashSignature{
	{
		needles: []string{"UnsupportedClassVersionError"},
		problem: func(int) Problem {
			return Problem{
				Title: "The server needs a newer Java",
				Hint:  "This version needs a newer Java than the one it was started with. Try updating or reinstalling the server.",
			}
		},
	},
	{
		needles: []string{"Address already in use", "FAILED TO BIND TO PORT", "Perhaps a server is already running on that port"},
		problem: func(port int) Problem {
			return Problem{
				Title: fmt.Sprintf("Port %d is already in use", port),
				Hint:  "Another server or program is using this port. Stop it, or change this server's port.",
			}
		},
	},
	{
		needles: []string{"Could not reserve enough space", "Invalid maximum heap size", "Invalid initial heap size", "exceeds the maximum representable size"},
		problem: func(int) Problem {
			return Problem{
				Title: "Not enough memory to start",
				Hint:  "Lower this server's Max RAM so it fits in your computer's free memory.",
			}
		},
	},
	{
		needles: []string{"OutOfMemoryError"},
		problem: func(int) Problem {
			return Problem{
				Title: "The server ran out of memory",
				Hint:  "Raise its Max RAM, or remove heavy plugins and mods.",
			}
		},
	},
	{
		needles: []string{"Error loading plugin", "Could not load 'plugins/"},
		problem: func(int) Problem {
			return Problem{
				Title: "A plugin failed to load",
				Hint:  "Start in Safe Mode to boot without plugins, then remove the plugin named in the console.",
			}
		},
	},
	{
		needles: []string{"No space left on device"},
		problem: func(int) Problem {
			return Problem{
				Title: "The disk is full",
				Hint:  "Free up disk space, then start the server again.",
			}
		},
	},
}

func diagnoseCrash(logLines []string, exitErr error, port int) *Problem {
	for _, sig := range crashSignatures {
		for _, line := range logLines {
			for _, needle := range sig.needles {
				if strings.Contains(line, needle) {
					p := sig.problem(port)
					p.Detail = truncateDetail(strings.TrimSpace(line))
					return &p
				}
			}
		}
	}
	p := &Problem{
		Title: "The server stopped unexpectedly",
		Hint:  "Open the Console or Logs tab to see why.",
	}
	if exitErr != nil {
		p.Detail = truncateDetail(exitErr.Error())
		if strings.Contains(exitErr.Error(), "signal: killed") {
			p.Title = "The system stopped the server"
			p.Hint = "Your computer probably ran out of memory. Lower Max RAM or close other programs."
		}
	}
	return p
}

func lastLogLines(entries []ConsoleLogEntry, n int) []string {
	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.Line
	}
	return lines
}

func truncateDetail(s string) string {
	if len(s) > problemDetailMax {
		return s[:problemDetailMax] + "..."
	}
	return s
}

func crashRestartDecision(times []time.Time, now time.Time) (restart bool, kept []time.Time) {
	for _, t := range times {
		if now.Sub(t) < crashRestartWindow {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	return len(kept) <= crashRestartLimit, kept
}
