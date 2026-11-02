// Package software kennt die unterstützten Server-Arten (Paper, Velocity, Fabric, Forge ...), findet ihre Versionen und
// Download-Adressen und bestimmt die passende Java-Version.
package software

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ID kennzeichnet eine Server-Art.
type ID string

const (
	Paper    ID = "paper"
	Folia    ID = "folia"
	Purpur   ID = "purpur"
	Vanilla  ID = "vanilla"
	Leaf     ID = "leaf"
	Fabric   ID = "fabric"
	NeoForge ID = "neoforge"
	Forge    ID = "forge"
	Velocity ID = "velocity"
)

// Kind sagt, wofür eine Software gedacht ist.
type Kind string

const (
	KindServer Kind = "server"
	KindProxy  Kind = "proxy"
)

// Extensions nennt, womit sich die Software erweitern lässt.
type Extensions string

const (
	ExtPlugins Extensions = "plugins"
	ExtMods    Extensions = "mods"
	ExtNone    Extensions = "none"
)

// Info beschreibt eine Server-Art für Oberfläche und Anlegen.
type Info struct {
	ID          ID         `json:"id"`
	Name        string     `json:"name"`
	Kind        Kind       `json:"kind"`
	Extensions  Extensions `json:"extensions"`
	DefaultPort int        `json:"defaultPort"`
	// Startup ist die Standard-Startzeile. {{MEMORY}} wird durch den Heap in MB ersetzt, {{JAR}} durch die Jar-Datei.
	Startup string `json:"startup"`
	// Command ist der Befehl, mit dem der Server sauber herunterfährt.
	StopCommand string `json:"stopCommand"`
	// ReadyPattern erkennt in der Konsole, dass der Server bereit ist.
	ReadyPattern string `json:"readyPattern"`
	Description  string `json:"description,omitempty"`
	CanBeBackend bool   `json:"canBeBackend"`
	CanVoice     bool   `json:"canVoice"`
	Custom       bool   `json:"custom,omitempty"`
}

const javaStartup = "exec java -Xms{{MEMORY}}M -Xmx{{MEMORY}}M -XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 -XX:+DisableExplicitGC -Dterminal.jline=false -Dterminal.ansi=true -jar {{JAR}}"

// installerStartup startet Forge und NeoForge: Beim ersten Start läuft der Installer, danach run.sh mit den Speicherwerten.
const installerStartup = "if [ ! -f run.sh ]; then java -jar {{JAR}} --installServer && if [ -f run.sh ]; then sed -i 's/^java/exec java/' run.sh 2>/dev/null || true; fi; fi; echo '-Xms{{MEMORY}}M -Xmx{{MEMORY}}M -XX:+UseG1GC -XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=200 -XX:+DisableExplicitGC -Dterminal.jline=false -Dterminal.ansi=true' > user_jvm_args.txt; exec sh run.sh nogui"

func isBuiltinProvider(t string) bool {
	switch ID(t) {
	case Paper, Purpur, Folia, Leaf, Fabric, NeoForge, Forge, Vanilla, Velocity:
		return true
	default:
		return false
	}
}

// Catalog liefert alle unterstützten Server-Arten.
func Catalog() []Info {
	packs := GetPackManager().All()
	out := make([]Info, 0, len(packs))
	for _, p := range packs {
		out = append(out, p.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup findet eine Server-Art.
func Lookup(id ID) (Info, bool) {
	if pack, ok := GetPackManager().Get(id); ok {
		return pack.Info(), true
	}
	return Info{}, false
}

// Artifact ist die herunterzuladende Server-Datei.
type Artifact struct {
	URL      string `json:"url"`
	FileName string `json:"fileName"`
	SHA256   string `json:"sha256,omitempty"`
	SHA1     string `json:"sha1,omitempty"`
}

// Resolver fragt die Anbieter ab. Die Adressen sind austauschbar, damit sich alles ohne Netz testen lässt.
type Resolver struct {
	HTTP           *http.Client
	FillBase       string
	MojangManifest string
	FabricMeta     string
	PurpurBase     string
	NeoForgeMaven  string
	LeafBase       string
	ForgePromos    string
	ForgeMaven     string
}

// NewResolver nutzt die öffentlichen Adressen der Anbieter.
func NewResolver() *Resolver {
	return &Resolver{
		HTTP:           &http.Client{Timeout: 30 * time.Second},
		FillBase:       "https://fill.papermc.io",
		MojangManifest: "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json",
		FabricMeta:     "https://meta.fabricmc.net",
		PurpurBase:     "https://api.purpurmc.org",
		NeoForgeMaven:  "https://maven.neoforged.net/releases/net/neoforged/neoforge",
		LeafBase:       "https://api.leafmc.one",
		ForgePromos:    "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json",
		ForgeMaven:     "https://maven.minecraftforge.net/net/minecraftforge/forge",
	}
}

func (r *Resolver) getJSON(ctx context.Context, address string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Lodestone")
	response, err := r.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s antwortet mit Status %d", request.URL.Host, response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(into)
}

func (r *Resolver) getXML(ctx context.Context, address string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Lodestone")
	response, err := r.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s antwortet mit Status %d", request.URL.Host, response.StatusCode)
	}
	return xml.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(into)
}

func (r *Resolver) getText(ctx context.Context, address string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "Lodestone")
	response, err := r.HTTP.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s antwortet mit Status %d", request.URL.Host, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ErrNotFound bedeutet, dass es die gewünschte Version nicht gibt.
var ErrNotFound = errors.New("diese Version gibt es nicht")

// ---------------------------------------------------------------- Versionen

// Versions liefert die verfügbaren Versionen, die neueste zuerst. Vorabversionen nur mit prerelease.
func (r *Resolver) Versions(ctx context.Context, id ID, prerelease bool) ([]string, error) {
	if pack, ok := GetPack(id); ok {
		if pack.Meta.Provider.Type == "static" {
			return pack.Meta.Provider.Versions, nil
		}
		if pack.Meta.Provider.Type == "direct" {
			return pack.Meta.Provider.Versions, nil
		}
		if pack.Meta.Provider.Type != "" && isBuiltinProvider(pack.Meta.Provider.Type) && ID(pack.Meta.Provider.Type) != id {
			return r.Versions(ctx, ID(pack.Meta.Provider.Type), prerelease)
		}
	}
	var versions []string
	switch id {
	case Paper, Folia, Velocity:
		var project struct {
			Versions map[string][]string `json:"versions"`
		}
		if err := r.getJSON(ctx, fmt.Sprintf("%s/v3/projects/%s", r.FillBase, id), &project); err != nil {
			return nil, err
		}
		groups := make([]string, 0, len(project.Versions))
		for group := range project.Versions {
			groups = append(groups, group)
		}
		sort.Slice(groups, func(i, j int) bool { return CompareVersions(groups[i], groups[j]) > 0 })
		for _, group := range groups {
			versions = append(versions, project.Versions[group]...)
		}
	case Purpur:
		var project struct {
			Versions []string `json:"versions"`
		}
		if err := r.getJSON(ctx, r.PurpurBase+"/v2/purpur", &project); err != nil {
			return nil, err
		}
		versions = project.Versions
		sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
	case Leaf:
		var project struct {
			Versions []string `json:"versions"`
		}
		if err := r.getJSON(ctx, r.LeafBase+"/v2/projects/leaf", &project); err != nil {
			return nil, err
		}
		versions = project.Versions
		sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
	case Forge:
		promos, err := r.forgePromos(ctx)
		if err != nil {
			return nil, err
		}
		for mc := range promos {
			versions = append(versions, mc)
		}
		sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
		return versions, nil
	case Vanilla:
		manifest, err := r.manifest(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range manifest.Versions {
			if entry.Type == "release" || (prerelease && entry.Type == "snapshot") {
				versions = append(versions, entry.ID)
			}
		}
		return versions, nil
	case Fabric:
		var games []versionEntry
		if err := r.getJSON(ctx, r.FabricMeta+"/v2/versions/game", &games); err != nil {
			return nil, err
		}
		for _, game := range games {
			if game.Stable || prerelease {
				versions = append(versions, game.Version)
			}
		}
		return versions, nil
	case NeoForge:
		var meta struct {
			Versioning struct {
				Versions []string `xml:"versions>version"`
			} `xml:"versioning"`
		}
		if err := r.getXML(ctx, r.NeoForgeMaven+"/maven-metadata.xml", &meta); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, v := range meta.Versioning.Versions {
			if !prerelease && isPrerelease(v) {
				continue
			}
			mc := NeoForgeToMinecraft(v)
			if mc != "" && !seen[mc] {
				seen[mc] = true
				versions = append(versions, mc)
			}
		}
		sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
		return versions, nil
	default:
		return nil, fmt.Errorf("unbekannte Software: %s", id)
	}
	if id == Velocity {
		return velocityOrder(versions), nil
	}
	if prerelease {
		return versions, nil
	}
	stable := versions[:0:0]
	for _, version := range versions {
		if !isPrerelease(version) {
			stable = append(stable, version)
		}
	}
	return stable, nil
}

// velocityOrder stellt die Releases der stabilen Reihe 3.x nach vorn: Velocity 4.x ist noch Beta und braucht Java 25.
// Fill nennt bei Velocity auch die Releases "SNAPSHOT", deshalb filtert isPrerelease hier nicht.
func velocityOrder(versions []string) []string {
	var first, rest []string
	for _, version := range versions {
		if strings.HasPrefix(version, "3.") && !strings.Contains(version, "SNAPSHOT") {
			first = append(first, version)
		} else {
			rest = append(rest, version)
		}
	}
	return append(first, rest...)
}

func isPrerelease(version string) bool {
	lower := strings.ToLower(version)
	for _, marker := range []string{"-rc", "-pre", "snapshot", "-beta", "-alpha"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// CompareVersions vergleicht Versionsnummern wie 1.21.8 und 26.3 zahlenweise: positiv, wenn a neuer ist als b.
func CompareVersions(a, b string) int {
	pa, pb := numericParts(a), numericParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return strings.Compare(b, a) // bei gleichen Zahlen: die ohne Zusatz ("1.21") ist neuer als "1.21-rc-1"
}

func numericParts(version string) []int {
	base := version
	if cut := strings.IndexAny(base, "-+ "); cut >= 0 {
		base = base[:cut]
	}
	var parts []int
	for _, piece := range strings.Split(base, ".") {
		number, err := strconv.Atoi(piece)
		if err != nil {
			break
		}
		parts = append(parts, number)
	}
	return parts
}

// NeoForgeToMinecraft übersetzt eine NeoForge-Version (z. B. 21.4.161 oder 26.1.2.125) in die zugehörige
// Minecraft-Spielversion (1.21.4 bzw. 26.1.2). Ist version schon eine Minecraft-Version, bleibt sie unverändert.
func NeoForgeToMinecraft(version string) string {
	parts := numericParts(version)
	if len(parts) == 0 {
		return version
	}
	if parts[0] == 20 || parts[0] == 21 {
		minor := 0
		if len(parts) > 1 {
			minor = parts[1]
		}
		if minor == 0 {
			return fmt.Sprintf("1.%d", parts[0])
		}
		return fmt.Sprintf("1.%d.%d", parts[0], minor)
	}
	if parts[0] >= 26 {
		if len(parts) > 2 && parts[2] > 0 {
			return fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2])
		}
		if len(parts) > 1 {
			return fmt.Sprintf("%d.%d", parts[0], parts[1])
		}
		return fmt.Sprintf("%d", parts[0])
	}
	return version
}

// ---------------------------------------------------------------- Auflösen

// Resolve findet die Download-Datei zu einer Version.
func (r *Resolver) Resolve(ctx context.Context, id ID, version string) (Artifact, error) {
	if version == "" || strings.ContainsAny(version, "/\\ ?#") {
		return Artifact{}, fmt.Errorf("ungültige Version: %q", version)
	}
	if pack, ok := GetPack(id); ok {
		if pack.Meta.Provider.DownloadURL != "" {
			address := strings.ReplaceAll(pack.Meta.Provider.DownloadURL, "{{VERSION}}", version)
			fileName := pack.Meta.Provider.FileName
			if fileName == "" {
				fileName = fmt.Sprintf("%s-%s.jar", id, version)
			}
			fileName = strings.ReplaceAll(fileName, "{{VERSION}}", version)
			return Artifact{URL: address, FileName: fileName}, nil
		}
		if pack.Meta.Provider.Type != "" && isBuiltinProvider(pack.Meta.Provider.Type) && ID(pack.Meta.Provider.Type) != id {
			return r.Resolve(ctx, ID(pack.Meta.Provider.Type), version)
		}
	}
	switch id {
	case Paper, Folia, Velocity:
		var build struct {
			Downloads map[string]struct {
				Name      string `json:"name"`
				URL       string `json:"url"`
				Checksums struct {
					SHA256 string `json:"sha256"`
				} `json:"checksums"`
			} `json:"downloads"`
		}
		address := fmt.Sprintf("%s/v3/projects/%s/versions/%s/builds/latest", r.FillBase, id, url.PathEscape(version))
		if err := r.getJSON(ctx, address, &build); err != nil {
			return Artifact{}, err
		}
		download, ok := build.Downloads["server:default"]
		if !ok || download.URL == "" {
			return Artifact{}, fmt.Errorf("für %s %s gibt es keinen Download", id, version)
		}
		return Artifact{URL: download.URL, FileName: download.Name, SHA256: download.Checksums.SHA256}, nil
	case Leaf:
		var builds struct {
			Builds []int `json:"builds"`
		}
		base := fmt.Sprintf("%s/v2/projects/leaf/versions/%s", r.LeafBase, url.PathEscape(version))
		if err := r.getJSON(ctx, base, &builds); err != nil {
			return Artifact{}, err
		}
		if len(builds.Builds) == 0 {
			return Artifact{}, ErrNotFound
		}
		latest := slices.Max(builds.Builds)
		var build struct {
			Downloads struct {
				Primary struct {
					Name   string `json:"name"`
					SHA256 string `json:"sha256"`
				} `json:"primary"`
			} `json:"downloads"`
		}
		if err := r.getJSON(ctx, fmt.Sprintf("%s/builds/%d", base, latest), &build); err != nil {
			return Artifact{}, err
		}
		name := build.Downloads.Primary.Name
		if name == "" || strings.ContainsAny(name, "/\\") {
			return Artifact{}, fmt.Errorf("für Leaf %s gibt es keinen Download", version)
		}
		return Artifact{URL: fmt.Sprintf("%s/builds/%d/downloads/%s", base, latest, url.PathEscape(name)), FileName: name, SHA256: build.Downloads.Primary.SHA256}, nil
	case Forge:
		promos, err := r.forgePromos(ctx)
		if err != nil {
			return Artifact{}, err
		}
		forge, ok := promos[version]
		if !ok {
			return Artifact{}, ErrNotFound
		}
		full := version + "-" + forge
		address := fmt.Sprintf("%s/%s/forge-%s-installer.jar", r.ForgeMaven, url.PathEscape(full), url.PathEscape(full))
		artifact := Artifact{URL: address, FileName: fmt.Sprintf("forge-%s-installer.jar", full)}
		sum, err := r.getText(ctx, address+".sha256")
		if err != nil {
			return Artifact{}, fmt.Errorf("Prüfsumme für Forge %s nicht abrufbar: %w", full, err)
		}
		if fields := strings.Fields(sum); len(fields) > 0 {
			artifact.SHA256 = fields[0]
		}
		return artifact, nil
	case Purpur:
		return Artifact{URL: fmt.Sprintf("%s/v2/purpur/%s/latest/download", r.PurpurBase, url.PathEscape(version)), FileName: "purpur-" + version + ".jar"}, nil
	case Vanilla:
		detail, err := r.versionDetail(ctx, version)
		if err != nil {
			return Artifact{}, err
		}
		if detail.Downloads.Server.URL == "" {
			return Artifact{}, fmt.Errorf("für Vanilla %s gibt es keinen Server-Download", version)
		}
		return Artifact{URL: detail.Downloads.Server.URL, FileName: "minecraft_server." + version + ".jar", SHA1: detail.Downloads.Server.SHA1}, nil
	case Fabric:
		var loaders []versionEntry
		if err := r.getJSON(ctx, r.FabricMeta+"/v2/versions/loader", &loaders); err != nil {
			return Artifact{}, err
		}
		var installers []versionEntry
		if err := r.getJSON(ctx, r.FabricMeta+"/v2/versions/installer", &installers); err != nil {
			return Artifact{}, err
		}
		loader, installer := firstStable(loaders), firstStable(installers)
		if loader == "" || installer == "" {
			return Artifact{}, errors.New("Fabric meldet keine stabile Loader- oder Installer-Version")
		}
		address := fmt.Sprintf("%s/v2/versions/loader/%s/%s/%s/server/jar", r.FabricMeta, url.PathEscape(version), loader, installer)
		return Artifact{URL: address, FileName: fmt.Sprintf("fabric-server-mc.%s-loader.%s-launcher.%s.jar", version, loader, installer)}, nil
	case NeoForge:
		var meta struct {
			Versioning struct {
				Versions []string `xml:"versions>version"`
			} `xml:"versioning"`
		}
		if err := r.getXML(ctx, r.NeoForgeMaven+"/maven-metadata.xml", &meta); err != nil {
			return Artifact{}, err
		}
		var target string
		for _, v := range meta.Versioning.Versions {
			if v == version {
				target = v
				break
			}
		}
		if target == "" {
			for _, v := range meta.Versioning.Versions {
				if NeoForgeToMinecraft(v) == version && !isPrerelease(v) {
					if target == "" || CompareVersions(v, target) > 0 {
						target = v
					}
				}
			}
		}
		if target == "" {
			for _, v := range meta.Versioning.Versions {
				if NeoForgeToMinecraft(v) == version {
					if target == "" || CompareVersions(v, target) > 0 {
						target = v
					}
				}
			}
		}
		if target == "" {
			return Artifact{}, ErrNotFound
		}
		address := fmt.Sprintf("%s/%s/neoforge-%s-installer.jar", r.NeoForgeMaven, url.PathEscape(target), url.PathEscape(target))
		artifact := Artifact{URL: address, FileName: fmt.Sprintf("neoforge-%s-installer.jar", target)}
		sum, err := r.getText(ctx, address+".sha256")
		if err != nil {
			return Artifact{}, fmt.Errorf("Prüfsumme für NeoForge %s nicht abrufbar: %w", target, err)
		}
		if fields := strings.Fields(sum); len(fields) > 0 {
			artifact.SHA256 = fields[0]
		}
		return artifact, nil
	}
	return Artifact{}, fmt.Errorf("unbekannte Software: %s", id)
}

// forgeMinMinecraft ist die älteste Minecraft-Version, die Lodestone mit Forge startet. Ab 1.18 legt der Installer
// run.sh und user_jvm_args.txt an; ältere Forge-Server starten anders.
const forgeMinMinecraft = "1.18"

// forgePromos liefert zu jeder unterstützten Minecraft-Version die Forge-Version: die empfohlene, sonst die neueste.
func (r *Resolver) forgePromos(ctx context.Context) (map[string]string, error) {
	var list struct {
		Promos map[string]string `json:"promos"`
	}
	if err := r.getJSON(ctx, r.ForgePromos, &list); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for key, forge := range list.Promos {
		mc, kind, ok := strings.Cut(key, "-")
		if !ok || (kind != "latest" && kind != "recommended") || forge == "" || len(numericParts(mc)) < 2 || strings.ContainsAny(mc, "_- ") {
			continue
		}
		if CompareVersions(mc, forgeMinMinecraft) < 0 {
			continue
		}
		if _, have := out[mc]; !have || kind == "recommended" {
			out[mc] = forge
		}
	}
	return out, nil
}

type versionEntry struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

func firstStable(entries []versionEntry) string {
	for _, entry := range entries {
		if entry.Stable {
			return entry.Version
		}
	}
	return ""
}

// ---------------------------------------------------------------- Mojang

type manifest struct {
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"versions"`
}

type versionDetail struct {
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
	Downloads struct {
		Server struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
}

func (r *Resolver) manifest(ctx context.Context) (manifest, error) {
	var result manifest
	err := r.getJSON(ctx, r.MojangManifest, &result)
	return result, err
}

func (r *Resolver) versionDetail(ctx context.Context, version string) (versionDetail, error) {
	list, err := r.manifest(ctx)
	if err != nil {
		return versionDetail{}, err
	}
	for _, entry := range list.Versions {
		if entry.ID == version {
			var detail versionDetail
			err := r.getJSON(ctx, entry.URL, &detail)
			return detail, err
		}
	}
	return versionDetail{}, ErrNotFound
}

// JavaMajor bestimmt die Java-Version, die Software und Minecraft-Version brauchen. Mojang nennt sie je Version;
// ist das nicht erreichbar, rechnet FallbackJava sie aus der Versionsnummer.
func (r *Resolver) JavaMajor(ctx context.Context, id ID, version string) int {
	if pack, ok := GetPack(id); ok {
		if pack.Meta.Java.Strategy == "fixed" && pack.Meta.Java.Major > 0 {
			return pack.Meta.Java.Major
		}
		if pack.Meta.Java.Strategy == "velocity" || id == Velocity {
			if parts := numericParts(version); len(parts) > 0 && parts[0] >= 4 {
				return 25
			}
			return 21
		}
		if pack.Meta.Provider.Type != "" && isBuiltinProvider(pack.Meta.Provider.Type) && ID(pack.Meta.Provider.Type) != id {
			return r.JavaMajor(ctx, ID(pack.Meta.Provider.Type), version)
		}
	}
	if id == Velocity {
		if parts := numericParts(version); len(parts) > 0 && parts[0] >= 4 {
			return 25
		}
		return 21
	}
	mc := version
	if id == NeoForge {
		mc = NeoForgeToMinecraft(version)
	}
	if detail, err := r.versionDetail(ctx, mc); err == nil && detail.JavaVersion.MajorVersion > 0 {
		return detail.JavaVersion.MajorVersion
	}
	return FallbackJava(mc)
}

// FallbackJava schätzt die Java-Version aus der Minecraft-Version.
func FallbackJava(version string) int {
	parts := numericParts(version)
	if len(parts) == 0 {
		return 21
	}
	if parts[0] >= 26 {
		return 25
	}
	var minor, patch int
	if len(parts) > 1 {
		minor = parts[1]
	}
	if len(parts) > 2 {
		patch = parts[2]
	}
	switch {
	case minor > 20 || (minor == 20 && patch >= 5):
		return 21
	case minor >= 17:
		return 17
	case minor >= 13:
		return 11
	default:
		return 8
	}
}

// ---------------------------------------------------------------- Download

// Download lädt die Datei nach dst. Prüfsummen werden verglichen, sofern bekannt; bei einem Fehler bleibt nichts liegen.
func (r *Resolver) Download(ctx context.Context, artifact Artifact, dst string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Lodestone")
	client := *r.HTTP
	client.Timeout = 0 // große Dateien dürfen länger dauern, der Kontext begrenzt
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Download antwortet mit Status %d", response.StatusCode)
	}
	temp, err := os.CreateTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	var sums []struct {
		name string
		want string
		hash hash.Hash
	}
	writers := []io.Writer{temp}
	if artifact.SHA256 != "" {
		h := sha256.New()
		sums = append(sums, struct {
			name string
			want string
			hash hash.Hash
		}{"SHA-256", artifact.SHA256, h})
		writers = append(writers, h)
	}
	if artifact.SHA1 != "" {
		h := sha1.New()
		sums = append(sums, struct {
			name string
			want string
			hash hash.Hash
		}{"SHA-1", artifact.SHA1, h})
		writers = append(writers, h)
	}
	if _, err := io.Copy(io.MultiWriter(writers...), response.Body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	for _, sum := range sums {
		if got := hex.EncodeToString(sum.hash.Sum(nil)); !strings.EqualFold(got, sum.want) {
			return fmt.Errorf("%s stimmt nicht (erwartet %s, erhalten %s)", sum.name, sum.want, got)
		}
	}
	return os.Rename(temp.Name(), dst)
}
