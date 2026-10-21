package minecraft

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func testJavaResolver(majors ...int) *javaRequirementResolver {
	r := &javaRequirementResolver{
		availableByMaj:  make(map[int]string),
		vanillaReqCache: make(map[string]int),
	}
	for _, major := range majors {
		r.availableByMaj[major] = bundledJDKPaths[major]
	}
	return r
}

func TestJavaResolverVelocity410(t *testing.T) {
	r := testJavaResolver(17, 21, 25)
	exec, required, selected, err := r.resolve("velocity", "4.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if required != 25 || selected != 25 || exec != "/opt/jdks/jdk25/bin/java" {
		t.Fatalf("Velocity 4.1.0: required=%d selected=%d exec=%s; want 25, 25, /opt/jdks/jdk25/bin/java", required, selected, exec)
	}
	t.Logf("Velocity 4.1.0: required=%d selected=%d exec=%s", required, selected, exec)
}

func TestJavaResolverVelocityVersions(t *testing.T) {
	tests := []struct {
		serverType string
		version    string
		major      int
	}{
		{"velocity", "3.3.0", 21},
		{"velocity", "3.5.1", 21},
		{"velocity", "3.6.0-SNAPSHOT", 21},
		{"velocity", "4.0.0", 25},
		{"velocity", "4.1.0-SNAPSHOT", 25},
		{" VeLoCiTy ", " 4.2.1-snapshot ", 25},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			exec, required, selected, err := testJavaResolver(17, 21, 25).resolve(tt.serverType, tt.version)
			if err != nil || required != tt.major || selected != tt.major || exec != bundledJDKPaths[tt.major] {
				t.Fatalf("required=%d selected=%d exec=%q err=%v; want Java %d", required, selected, exec, err, tt.major)
			}
		})
	}
}

func TestJavaResolverMinecraftPlatforms(t *testing.T) {
	for _, serverType := range []string{"paper", "spigot", "purpur", "folia", "fabric", "forge", "neoforge", " PaPeR "} {
		for _, version := range []struct {
			value string
			major int
		}{
			{"1.16.5", 17},
			{"1.20", 17},
			{"1.20.4", 17},
			{"1.20.6", 17},
			{"1.21", 21},
			{"1.21.10", 21},
			{" 1.22.1 ", 25},
			{"1.23.0", 25},
		} {
			t.Run(serverType+"/"+version.value, func(t *testing.T) {
				exec, required, selected, err := testJavaResolver(17, 21, 25).resolve(serverType, version.value)
				if err != nil || required != version.major || selected != version.major || exec != bundledJDKPaths[version.major] {
					t.Fatalf("required=%d selected=%d exec=%q err=%v; want Java %d", required, selected, exec, err, version.major)
				}
			})
		}
	}
}

func TestJavaResolverRejectsUnknownRequirements(t *testing.T) {
	tests := []struct {
		serverType string
		versions   []string
	}{
		{"velocity", []string{"", "latest", "1.1.9", "2.0.0", "5.0.0", "1.21.1", "4", "4.1", "4.x.0", "4.1.0.1", "4.1.0-unknown", "4.1.0-SNAPSHOT-extra", "4.1.-1", "+4.1.0", "nonsense"}},
		{"paper", []string{"", "latest", "4.1.0", "1.x.0", "1.21.x", "1.21.1.2", "1.21-SNAPSHOT", "1.-1", "1.21garbage", "1.21.1-52.0.0", "1.999999999999999999999999999999", "26.1", "nonsense"}},
		{"fabric", []string{"0.16.10"}},
		{"forge", []string{"52.0.0", "1.21.1-52.0.0"}},
		{"neoforge", []string{"21.1.80"}},
		{"unknown", []string{"1.21.1", "4.1.0"}},
		{"", []string{"1.21.1"}},
	}
	for _, tt := range tests {
		for _, version := range tt.versions {
			t.Run(tt.serverType+"/"+version, func(t *testing.T) {
				exec, required, selected, err := testJavaResolver(17, 21, 25).resolve(tt.serverType, version)
				if err == nil || exec != "" || required != 0 || selected != 0 {
					t.Fatalf("required=%d selected=%d exec=%q err=%v; want explicit error without a runtime", required, selected, exec, err)
				}
				if !strings.Contains(err.Error(), fmt.Sprintf("type=%q", tt.serverType)) || !strings.Contains(err.Error(), fmt.Sprintf("version=%q", version)) {
					t.Fatalf("error lacks type/version context: %v", err)
				}
			})
		}
	}
}

func TestJavaResolverAvailableRuntimes(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		majors   []int
		required int
		selected int
		wantErr  string
	}{
		{"none", "4.1.0", nil, 25, 0, "no bundled JDK runtimes available"},
		{"insufficient", "4.1.0", []int{17, 21}, 25, 0, "required Java 25 is not available"},
		{"higher compatible", "3.5.1", []int{25}, 21, 25, ""},
		{"lowest compatible", "3.5.1", []int{25, 17, 21}, 21, 21, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, required, selected, err := testJavaResolver(tt.majors...).resolve("velocity", tt.version)
			if required != tt.required || selected != tt.selected {
				t.Fatalf("required=%d selected=%d; want %d, %d", required, selected, tt.required, tt.selected)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || exec != "" {
					t.Fatalf("exec=%q err=%v; want %q without an executable", exec, err, tt.wantErr)
				}
			} else if err != nil || exec != bundledJDKPaths[tt.selected] {
				t.Fatalf("exec=%q err=%v; want %q", exec, err, bundledJDKPaths[tt.selected])
			}
		})
	}
}

type javaTestTransport func(*http.Request) (*http.Response, error)

func (f javaTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// These tests replace the default transport and must not run in parallel.
func mockJavaMetadata(t *testing.T, fn javaTestTransport) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = fn
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func javaMetadataResponse(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestJavaResolverVanillaMetadataAndCache(t *testing.T) {
	for _, version := range []string{"1.20.6", "26.1", "26w01a"} {
		t.Run(version, func(t *testing.T) {
			manifestCalls, metadataCalls := 0, 0
			mockJavaMetadata(t, func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case "https://launchermeta.mojang.com/mc/game/version_manifest.json":
					manifestCalls++
					return javaMetadataResponse(req, fmt.Sprintf(`{"versions":[{"id":%q,"url":"https://metadata.test/version.json"}]}`, version)), nil
				case "https://metadata.test/version.json":
					metadataCalls++
					return javaMetadataResponse(req, `{"javaVersion":{"majorVersion":25}}`), nil
				default:
					t.Fatalf("unexpected metadata URL: %s", req.URL)
					return nil, fmt.Errorf("unexpected URL")
				}
			})
			r := testJavaResolver(17, 21, 25)
			for attempt := 0; attempt < 2; attempt++ {
				exec, required, selected, err := r.resolve(" VaNiLlA ", " "+version+" ")
				if err != nil || required != 25 || selected != 25 || exec != bundledJDKPaths[25] {
					t.Fatalf("metadata requirement: required=%d selected=%d exec=%q err=%v", required, selected, exec, err)
				}
			}
			if manifestCalls != 1 || metadataCalls != 1 || r.vanillaReqCache[version] != 25 {
				t.Fatalf("cache: manifest calls=%d metadata calls=%d cache=%v", manifestCalls, metadataCalls, r.vanillaReqCache)
			}
		})
	}
}

func TestJavaResolverVanillaMetadataFailure(t *testing.T) {
	for _, failure := range []string{"network", "status", "invalid JSON", "missing version", "missing requirement"} {
		t.Run(failure, func(t *testing.T) {
			manifestCalls := 0
			mockJavaMetadata(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "metadata.test" {
					return javaMetadataResponse(req, `{"javaVersion":{"majorVersion":0}}`), nil
				}
				manifestCalls++
				switch failure {
				case "network":
					return nil, fmt.Errorf("metadata unavailable")
				case "status":
					resp := javaMetadataResponse(req, "")
					resp.StatusCode = http.StatusServiceUnavailable
					return resp, nil
				case "invalid JSON":
					return javaMetadataResponse(req, "not JSON"), nil
				case "missing version":
					return javaMetadataResponse(req, `{"versions":[]}`), nil
				default:
					return javaMetadataResponse(req, `{"versions":[{"id":"1.21.1","url":"https://metadata.test/version.json"}]}`), nil
				}
			})
			r := testJavaResolver(17, 21, 25)
			for attempt := 0; attempt < 2; attempt++ {
				exec, required, selected, err := r.resolve("vanilla", "1.21.1")
				if err != nil || required != 21 || selected != 21 || exec != bundledJDKPaths[21] {
					t.Fatalf("Minecraft fallback: required=%d selected=%d exec=%q err=%v", required, selected, exec, err)
				}
			}
			if manifestCalls != 2 || len(r.vanillaReqCache) != 0 {
				t.Fatalf("failed metadata must be retried: calls=%d cache=%v", manifestCalls, r.vanillaReqCache)
			}
			exec, required, selected, err := r.resolve("vanilla", "unrecognized")
			if err == nil || exec != "" || required != 0 || selected != 0 {
				t.Fatalf("unknown Vanilla identifier: required=%d selected=%d exec=%q err=%v", required, selected, exec, err)
			}
		})
	}
}

func TestJavaResolverVanillaUnresolvedAlias(t *testing.T) {
	for _, version := range []string{"", "latest", " LATEST "} {
		t.Run(version, func(t *testing.T) {
			mockJavaMetadata(t, func(req *http.Request) (*http.Response, error) {
				t.Fatalf("unresolved alias must not request metadata: %s", req.URL)
				return nil, fmt.Errorf("unexpected request")
			})
			exec, required, selected, err := testJavaResolver(17, 21, 25).resolve("vanilla", version)
			if err == nil || exec != "" || required != 0 || selected != 0 {
				t.Fatalf("required=%d selected=%d exec=%q err=%v; want unresolved-version error", required, selected, exec, err)
			}
		})
	}
}
