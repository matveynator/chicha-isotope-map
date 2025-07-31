// Package logger implements a per-trackID in-memory log buffer.
//
// Все подробности пишутся в буфер ПОКА идёт работа с файлом.
// ● Если произошла ошибка — буфер «проигрывается» и выводится итоговая ошибка.
// ● Если всё OK — буфер отбрасывается, пишем одну короткую строку.
//
// Потокобезопасность достигается через выделенный logger-goroutine и
// канал команд; никаких mutex-ов.
package logger

import (
	"bytes"
	"log"
	"strings"
	"time"
)

// --- типы команд ------------------------------------------------------------

type action int

const (
	actBegin action = iota
	actAppend
	actSuccess
	actFlushErr
)

type cmd struct {
	act      action
	trackID  string
	message  string        // для Append
	filename string        // для Success
	err      error         // для FlushErr
	when     time.Time     // отметка времени (для сортировки, если надо)
}

// --- публичные точки-входа (только отправляют в канал) ----------------------

var ch = make(chan cmd, 128) // буфер на случай всплесков

// Begin включает буферизацию для trackID.
func Begin(trackID string) { ch <- cmd{act: actBegin, trackID: trackID, when: time.Now()} }

// Append добавляет строку подробного лога.
func Append(trackID, msg string) {
	ch <- cmd{act: actAppend, trackID: trackID, message: msg, when: time.Now()}
}

// Success удаляет буфер и пишет короткую «успешную» строку.
func Success(trackID, filename string) {
	ch <- cmd{act: actSuccess, trackID: trackID, filename: filename, when: time.Now()}
}

// FlushError выводит накопленный буфер + финальную ошибку.
func FlushError(trackID string, err error) {
	ch <- cmd{act: actFlushErr, trackID: trackID, err: err, when: time.Now()}
}

// --- инициализация: стартуем goroutine -------------------------------------

func init() { go runloop() }

// --- приватная реализация ---------------------------------------------------

func runloop() {
	buffers := make(map[string]*bytes.Buffer)

	for c := range ch {
		switch c.act {
		case actBegin:
			buffers[c.trackID] = &bytes.Buffer{}

		case actAppend:
			if b := buffers[c.trackID]; b != nil {
				_, _ = b.WriteString(c.message + "\n")
			} else {
				log.Print(c.message) // нет буфера → пишем сразу
			}

		case actSuccess:
			log.Printf("[%-6s][Upload] ✔ processed %q", c.trackID, c.filename)
			delete(buffers, c.trackID)

		case actFlushErr:
			if b := buffers[c.trackID]; b != nil {
				lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
				for _, ln := range lines {
					log.Print(ln)
				}
				delete(buffers, c.trackID)
			}
			log.Printf("[%-6s][ERROR] %v", c.trackID, c.err)
		}
	}
}

