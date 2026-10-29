package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeProviders(t *testing.T) (*Resolver, *httptest.Server) {
	t.Helper()
	jar := []byte("not really a jar")
	sum := sha256.Sum256(jar)
	mux := http.NewServeMux()
	var server *httptest.Server

	mux.HandleFunc("/v3/projects/paper", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"versions":{"1.21":["1.21.8","1.21.7","1.21-rc-1"],"26.3":["26.3","26.3-rc-3"],"1.20":["1.20.6"]}}`)
	})
	mux.HandleFunc("/v3/projects/paper/versions/1.21.8/builds/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"downloads":{"server:default":{"name":"paper-1.21.8-60.jar","url":"%s/files/paper.jar","checksums":{"sha256":"%s"}}}}`, server.URL, hex.EncodeToString(sum[:]))
	})
	mux.HandleFunc("/v3/projects/paper/versions/9.9/builds/latest", http.NotFound)
	mux.HandleFunc("/files/paper.jar", func(w http.ResponseWriter, _ *http.Request) { w.Write(jar) })
	mux.HandleFunc("/mojang/manifest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"versions":[{"id":"1.21.8","type":"release","url":"%s/mojang/1.21.8"},{"id":"1.21.9-pre1","type":"snapshot","url":"x"}]}`, server.URL)
	})
	mux.HandleFunc("/mojang/1.21.8", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"javaVersion":{"majorVersion":21},"downloads":{"server":{"url":"http://example.invalid/server.jar","sha1":"abc"}}}`)
	})
	mux.HandleFunc("/fabric/v2/versions/loader", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"version":"0.21.0-beta","stable":false},{"version":"0.20.1","stable":true}]`)
	})
	mux.HandleFunc("/fabric/v2/versions/installer", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"version":"1.1.2","stable":true}]`)
	})
	mux.HandleFunc("/fabric/v2/versions/game", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"version":"26.4-snapshot-6","stable":false},{"version":"1.21.8","stable":true}]`)
	})
	mux.HandleFunc("/neoforge/maven-metadata.xml", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<metadata><versioning><versions>
<version>20.4.1-beta</version>
<version>20.4.253</version>
<version>21.1.265</version>
<version>21.4.161</version>
<version>26.3.0.99</version>
<version>26.4.0-alpha.1</version>
</versions></versioning></metadata>`)
	})
	mux.HandleFunc("/neoforge/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			fmt.Fprint(w, hex.EncodeToString(sum[:]))
			return
		}
		if strings.HasSuffix(r.URL.Path, "-installer.jar") {
			w.Write(jar)
			return
		}
		http.NotFound(w, r)
	})


	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return &Resolver{HTTP: server.Client(), FillBase: server.URL, MojangManifest: server.URL + "/mojang/manifest", FabricMeta: server.URL + "/fabric", PurpurBase: server.URL + "/purpur", NeoForgeMaven: server.URL + "/neoforge"}, server
}

func TestPaperVersionsSortedWithoutPrereleases(t *testing.T) {
	resolver, _ := fakeProviders(t)
	versions, err := resolver.Versions(context.Background(), Paper, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.3", "1.21.8", "1.21.7", "1.20.6"}
	if fmt.Sprint(versions) != fmt.Sprint(want) {
		t.Fatalf("Versionen = %v, erwartet %v", versions, want)
	}
	withPre, _ := resolver.Versions(context.Background(), Paper, true)
	if len(withPre) != 6 {
		t.Fatalf("mit Vorabversionen = %v", withPre)
	}
}

func TestResolveAndDownloadVerifiesChecksum(t *testing.T) {
	resolver, _ := fakeProviders(t)
	artifact, err := resolver.Resolve(context.Background(), Paper, "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "server.jar")
	if err := resolver.Download(context.Background(), artifact, dst); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "not really a jar" {
		t.Fatalf("Inhalt falsch: %q", data)
	}

	artifact.SHA256 = "0000"
	other := filepath.Join(t.TempDir(), "server.jar")
	if err := resolver.Download(context.Background(), artifact, other); err == nil {
		t.Fatal("eine falsche Prüfsumme muss den Download abbrechen")
	}
	if _, err := os.Stat(other); err == nil {
		t.Fatal("bei falscher Prüfsumme darf keine Datei liegen bleiben")
	}
}

func TestResolveUnknownVersionAndBadInput(t *testing.T) {
	resolver, _ := fakeProviders(t)
	if _, err := resolver.Resolve(context.Background(), Paper, "9.9"); err != ErrNotFound {
		t.Fatalf("erwartet ErrNotFound, erhalten %v", err)
	}
	for _, version := range []string{"", "../etc", "1 2", "1/2"} {
		if _, err := resolver.Resolve(context.Background(), Paper, version); err == nil {
			t.Fatalf("Version %q darf nicht akzeptiert werden", version)
		}
	}
}

func TestFabricUsesLatestStableLoader(t *testing.T) {
	resolver, server := fakeProviders(t)
	artifact, err := resolver.Resolve(context.Background(), Fabric, "1.21.8")
	if err != nil {
		t.Fatal(err)
	}
	want := server.URL + "/fabric/v2/versions/loader/1.21.8/0.20.1/1.1.2/server/jar"
	if artifact.URL != want {
		t.Fatalf("URL = %s, erwartet %s", artifact.URL, want)
	}
	versions, _ := resolver.Versions(context.Background(), Fabric, false)
	if fmt.Sprint(versions) != "[1.21.8]" {
		t.Fatalf("stabile Fabric-Versionen = %v", versions)
	}
}

func TestJavaMajor(t *testing.T) {
	resolver, _ := fakeProviders(t)
	ctx := context.Background()
	if got := resolver.JavaMajor(ctx, Paper, "1.21.8"); got != 21 {
		t.Fatalf("Mojang sagt 21, erhalten %d", got)
	}
	if got := resolver.JavaMajor(ctx, Velocity, "3.4.0-SNAPSHOT"); got != 21 {
		t.Fatalf("Velocity = %d", got)
	}
	cases := map[string]int{"26.3": 25, "1.21.8": 21, "1.20.5": 21, "1.20.4": 17, "1.18.2": 17, "1.17.1": 17, "1.16.5": 11, "1.12.2": 8, "": 21}
	for version, want := range cases {
		if got := FallbackJava(version); got != want {
			t.Errorf("FallbackJava(%q) = %d, erwartet %d", version, got, want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if CompareVersions("1.21.8", "1.21.7") <= 0 || CompareVersions("26.3", "1.21.8") <= 0 || CompareVersions("1.9", "1.10") >= 0 {
		t.Fatal("Zahlenvergleich falsch")
	}
	if CompareVersions("1.21", "1.21-rc-1") <= 0 {
		t.Fatal("Release muss neuer sein als Release-Candidate")
	}
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[ID]bool{}
	for _, info := range Catalog() {
		if seen[info.ID] {
			t.Fatalf("doppelte Software %s", info.ID)
		}
		seen[info.ID] = true
		if info.Startup == "" || info.StopCommand == "" || info.DefaultPort == 0 {
			t.Fatalf("%s ist unvollständig", info.ID)
		}
	}
	if proxy, _ := Lookup(Velocity); proxy.Kind != KindProxy || proxy.StopCommand != "end" {
		t.Fatal("Velocity muss ein Proxy sein und mit end stoppen")
	}
}

func TestVelocityJavaAndOrder(t *testing.T) {
	resolver := NewResolver()
	if got := resolver.JavaMajor(context.Background(), Velocity, "4.3.1"); got != 25 {
		t.Errorf("Velocity 4.x: Java %d, erwartet 25", got)
	}
	ordered := velocityOrder([]string{"4.3.2-SNAPSHOT", "4.3.1", "3.5.1", "3.6.0-SNAPSHOT", "3.4.0"})
	if ordered[0] != "3.5.1" || ordered[1] != "3.4.0" || ordered[2] != "4.3.2-SNAPSHOT" {
		t.Errorf("Reihenfolge: %v", ordered)
	}
}


func TestNeoForgeVersionsAndResolve(t *testing.T) {
	resolver, server := fakeProviders(t)
	ctx := context.Background()

	// Versions: stabil
	versions, err := resolver.Versions(ctx, NeoForge, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.3", "1.21.4", "1.21.1", "1.20.4"}
	if fmt.Sprint(versions) != fmt.Sprint(want) {
		t.Fatalf("NeoForge Versionen = %v, erwartet %v", versions, want)
	}

	// Versions: mit Vorabversionen
	withPre, err := resolver.Versions(ctx, NeoForge, true)
	if err != nil {
		t.Fatal(err)
	}
	wantPre := []string{"26.4", "26.3", "1.21.4", "1.21.1", "1.20.4"}
	if fmt.Sprint(withPre) != fmt.Sprint(wantPre) {
		t.Fatalf("NeoForge mit Vorabversionen = %v, erwartet %v", withPre, wantPre)
	}

	// Resolve über Minecraft-Version
	artifact, err := resolver.Resolve(ctx, NeoForge, "1.21.4")
	if err != nil {
		t.Fatal(err)
	}
	wantURL := server.URL + "/neoforge/21.4.161/neoforge-21.4.161-installer.jar"
	if artifact.URL != wantURL || artifact.FileName != "neoforge-21.4.161-installer.jar" {
		t.Fatalf("Artifact = %+v, erwartet URL %s", artifact, wantURL)
	}
	if artifact.SHA256 == "" {
		t.Fatal("NeoForge Artifact muss eine SHA256-Prüfsumme enthalten")
	}

	// Download mit Prüfsumme prüfen
	dst := filepath.Join(t.TempDir(), "installer.jar")
	if err := resolver.Download(ctx, artifact, dst); err != nil {
		t.Fatalf("NeoForge Download mit Prüfsumme fehlgeschlagen: %v", err)
	}

	// Resolve über direkte NeoForge-Version
	direct, err := resolver.Resolve(ctx, NeoForge, "20.4.253")
	if err != nil {
		t.Fatal(err)
	}
	if direct.FileName != "neoforge-20.4.253-installer.jar" || direct.SHA256 == "" {
		t.Fatalf("Direct Artifact = %+v", direct)
	}

	// Resolve unbekannte Version
	if _, err := resolver.Resolve(ctx, NeoForge, "1.12.2"); err != ErrNotFound {
		t.Fatalf("erwartet ErrNotFound, erhalten %v", err)
	}
}

func TestNeoForgeToMinecraft(t *testing.T) {
	cases := map[string]string{
		"20.4.253":   "1.20.4",
		"21.1.265":   "1.21.1",
		"21.0.167":   "1.21",
		"21.4.161":   "1.21.4",
		"26.1.2.125": "26.1.2",
		"26.3.0.99":  "26.3",
		"1.21.4":     "1.21.4",
		"26.3":       "26.3",
	}
	for input, want := range cases {
		if got := NeoForgeToMinecraft(input); got != want {
			t.Errorf("NeoForgeToMinecraft(%q) = %q, erwartet %q", input, got, want)
		}
	}
}

func TestNeoForgeJavaMajor(t *testing.T) {
	resolver, _ := fakeProviders(t)
	ctx := context.Background()
	if got := resolver.JavaMajor(ctx, NeoForge, "21.4.161"); got != 21 {
		t.Errorf("NeoForge 21.4.161: Java %d, erwartet 21", got)
	}
	if got := resolver.JavaMajor(ctx, NeoForge, "20.4.253"); got != 17 {
		t.Errorf("NeoForge 20.4.253: Java %d, erwartet 17", got)
	}
	if got := resolver.JavaMajor(ctx, NeoForge, "26.3.0.99"); got != 25 {
		t.Errorf("NeoForge 26.3.0.99: Java %d, erwartet 25", got)
	}
}
