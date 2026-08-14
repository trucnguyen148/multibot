// Command export dumps every session's chat_transcript from sessions.db into
// one <user_id>.txt file per session, one "Sender: text" line per message.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"

	_ "modernc.org/sqlite"
)

type chatMessage struct {
	Sender    string    `json:"sender"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func main() {
	dbPath := flag.String("db", "sessions.db", "path to sessions.db")
	outDir := flag.String("out", "transcripts", "directory to write <user_id>.txt files into")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	rows, err := db.Query(`SELECT user_id, chat_transcript FROM sessions`)
	if err != nil {
		log.Fatalf("query sessions: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID string
		var chatTranscript sql.NullString
		if err := rows.Scan(&userID, &chatTranscript); err != nil {
			log.Fatalf("scan session row: %v", err)
		}
		var messages []chatMessage
		if chatTranscript.Valid && chatTranscript.String != "" {
			if err := json.Unmarshal([]byte(chatTranscript.String), &messages); err != nil {
				log.Printf("skipping session %s: invalid chat_transcript: %v", userID, err)
				continue
			}
		}

		outPath := filepath.Join(*outDir, unsafeFilenameChars.ReplaceAllString(userID, "_")+".txt")
		if err := writeTranscript(outPath, messages); err != nil {
			log.Fatalf("write %s: %v", outPath, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("iterate session rows: %v", err)
	}

	fmt.Printf("wrote %d transcript(s) to %s\n", count, *outDir)
}

func writeTranscript(path string, messages []chatMessage) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, m := range messages {
		if _, err := fmt.Fprintf(f, "%s: %s\n", m.Sender, m.Text); err != nil {
			return err
		}
	}
	return nil
}
