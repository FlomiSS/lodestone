package node

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// AvailableMemory liest MemAvailable des Hosts (Bytes). Unter Linux ist /proc/meminfo auch im Container die des Hosts.
// 0 heißt: unbekannt.
func AvailableMemory() int64 {
	return readMeminfo("/proc/meminfo", "MemAvailable")
}

func readMeminfo(path, key string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, key+":"))
		if len(fields) == 0 {
			return 0
		}
		kilobytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return 0
		}
		return kilobytes * 1024
	}
	return 0
}
