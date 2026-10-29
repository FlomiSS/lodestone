package node

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/lodestone-mc/lodestone/internal/software"
)

// PlayerInfo ist der Spielerstand eines laufenden Servers.
type PlayerInfo struct {
	Online    int      `json:"online"`
	Max       int      `json:"max"`
	Names     []string `json:"names"`
	Version   string   `json:"version"`
	MOTD      string   `json:"motd"`
	LatencyMs int64    `json:"latencyMs"`
}

type statusResponse struct {
	Version struct {
		Name string `json:"name"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
		} `json:"sample"`
	} `json:"players"`
	Description json.RawMessage `json:"description"`
}

// Players fragt den Server per Minecraft-Statusabfrage (wie die Serverliste im Spiel). Bei einem Server kommt die
// vollständige Liste über den Befehl "list", falls mehr Spieler online sind, als die Abfrage nennt.
func (s *Server) Players(ctx context.Context) (PlayerInfo, error) {
	if state := s.View().State; state != StateRunning {
		return PlayerInfo{}, ErrBadState
	}
	spec := s.snapshot()
	info, _ := software.Lookup(spec.Software)
	port := info.DefaultPort
	var candidates []string
	if s.m.cfg.Network != "" {
		candidates = append(candidates, fmt.Sprintf("lodestone-%s:%d", spec.ID, port))
	}
	for _, mapping := range spec.Ports {
		if mapping.Container == port && mapping.Protocol == "tcp" && mapping.Host > 0 {
			candidates = append(candidates, fmt.Sprintf("127.0.0.1:%d", mapping.Host))
		}
	}
	var lastErr error = errors.New("keine Adresse zum Abfragen")
	for _, address := range candidates {
		status, latency, err := queryStatus(ctx, address)
		if err != nil {
			lastErr = err
			continue
		}
		result := PlayerInfo{Online: status.Players.Online, Max: status.Players.Max, Version: status.Version.Name, MOTD: motdText(status.Description), LatencyMs: latency.Milliseconds(), Names: []string{}}
		for _, player := range status.Players.Sample {
			result.Names = append(result.Names, player.Name)
		}
		if result.Online > len(result.Names) && info.Kind == software.KindServer {
			if names := s.listNames(ctx); len(names) > 0 {
				result.Names = names
			}
		}
		return result, nil
	}
	return PlayerInfo{}, lastErr
}

var (
	ansi       = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	listOnline = regexp.MustCompile(`(?i)players online:\s*(.*)$`)
)

// listNames schickt "list" und liest die Antwort aus der Konsole.
func (s *Server) listNames(ctx context.Context) []string {
	events, cancel, _, _ := s.Subscribe(0)
	defer cancel()
	if s.Command("list") != nil {
		return nil
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if event.Type != "line" {
				continue
			}
			text := ansi.ReplaceAllString(event.Line.Text, "")
			if match := listOnline.FindStringSubmatch(text); match != nil {
				var names []string
				for _, name := range strings.Split(match[1], ",") {
					if name = strings.TrimSpace(name); name != "" {
						names = append(names, name)
					}
				}
				return names
			}
		case <-timeout:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func motdText(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var component struct {
		Text  string            `json:"text"`
		Extra []json.RawMessage `json:"extra"`
	}
	if json.Unmarshal(raw, &component) != nil {
		return ""
	}
	var out strings.Builder
	out.WriteString(component.Text)
	for _, extra := range component.Extra {
		out.WriteString(motdText(extra))
	}
	return out.String()
}

// ---------------------------------------------------------------- Minecraft Server List Ping

func writeVarInt(w io.Writer, value int) {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		_, _ = w.Write([]byte{b})
		if value == 0 {
			return
		}
	}
}

func readVarInt(r io.ByteReader) (int, error) {
	value := 0
	for shift := 0; shift < 35; shift += 7 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int(b&0x7f) << shift
		if b&0x80 == 0 {
			return value, nil
		}
	}
	return 0, errors.New("ungültiger VarInt")
}

func packet(id int, body []byte) []byte {
	var inner []byte
	buffer := &bytesWriter{}
	writeVarInt(buffer, id)
	inner = append(buffer.data, body...)
	frame := &bytesWriter{}
	writeVarInt(frame, len(inner))
	return append(frame.data, inner...)
}

type bytesWriter struct{ data []byte }

func (b *bytesWriter) Write(p []byte) (int, error) { b.data = append(b.data, p...); return len(p), nil }

func queryStatus(ctx context.Context, address string) (statusResponse, time.Duration, error) {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return statusResponse{}, 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))

	host, portText, _ := net.SplitHostPort(address)
	var port uint16
	_, _ = fmt.Sscanf(portText, "%d", &port)
	body := &bytesWriter{}
	writeVarInt(body, 765)
	writeVarInt(body, len(host))
	body.data = append(body.data, host...)
	body.data = binary.BigEndian.AppendUint16(body.data, port)
	writeVarInt(body, 1)
	started := time.Now()
	if _, err := conn.Write(packet(0x00, body.data)); err != nil {
		return statusResponse{}, 0, err
	}
	if _, err := conn.Write(packet(0x00, nil)); err != nil {
		return statusResponse{}, 0, err
	}
	reader := bufio.NewReader(conn)
	length, err := readVarInt(reader)
	if err != nil || length <= 0 || length > 1<<20 {
		return statusResponse{}, 0, fmt.Errorf("ungültige Antwort des Servers")
	}
	frame := make([]byte, length)
	if _, err := io.ReadFull(reader, frame); err != nil {
		return statusResponse{}, 0, err
	}
	latency := time.Since(started)
	payload := &byteReader{data: frame}
	if id, err := readVarInt(payload); err != nil || id != 0x00 {
		return statusResponse{}, 0, errors.New("unerwartete Antwort des Servers")
	}
	size, err := readVarInt(payload)
	if err != nil || size < 0 || size > len(payload.data)-payload.pos {
		return statusResponse{}, 0, errors.New("ungültige Antwort des Servers")
	}
	var status statusResponse
	if err := json.Unmarshal(payload.data[payload.pos:payload.pos+size], &status); err != nil {
		return statusResponse{}, 0, err
	}
	return status, latency, nil
}

type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) ReadByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}
