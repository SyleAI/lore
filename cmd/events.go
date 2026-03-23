package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/spf13/cobra"
)

var (
	eventsFollow bool
	eventsFilter string
	eventsAll    bool
	eventsLast   int
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show or stream lore events",
	Long: `Show events emitted by lore commands.

By default, shows the last 50 events. Use --follow to tail the log in real time,
or --all to print the full history. Use --filter to match a specific event type prefix
(e.g. "ticket" matches ticket.created, ticket.claimed, etc.).`,
	RunE: runEvents,
}

func init() {
	eventsCmd.Flags().BoolVarP(&eventsFollow, "follow", "f", false, "stream new events as they arrive")
	eventsCmd.Flags().StringVar(&eventsFilter, "filter", "", "only show events whose type starts with this prefix")
	eventsCmd.Flags().BoolVar(&eventsAll, "all", false, "print all events from the beginning of the log")
	eventsCmd.Flags().IntVar(&eventsLast, "last", 50, "number of recent events to show (ignored with --all or --follow)")
	rootCmd.AddCommand(eventsCmd)
}

func runEvents(cmd *cobra.Command, _ []string) error {
	loreDir := loreDirFromContext(cmd.Context())
	if loreDir == "" {
		return fmt.Errorf("lore events: not inside a git repository with lore initialized")
	}

	logPath := loreDir + "/events.log"

	f, err := os.Open(logPath)
	if os.IsNotExist(err) {
		if eventsFollow {
			fmt.Fprintln(os.Stderr, "waiting for events...")
			return tailEvents(logPath, eventsFilter)
		}
		fmt.Println("no events yet (run some lore commands first)")
		return nil
	}
	if err != nil {
		return fmt.Errorf("lore events: %w", err)
	}
	defer f.Close()

	if eventsAll || eventsFollow {
		if err := printLines(f, eventsFilter); err != nil {
			return err
		}
	} else {
		// Collect last N matching lines.
		if err := printLastN(f, eventsFilter, eventsLast); err != nil {
			return err
		}
	}

	if !eventsFollow {
		return nil
	}

	// Seek to end, then tail.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("lore events: seek: %w", err)
	}
	return tailFile(f, eventsFilter)
}

// printLines reads all lines from r and prints matching events.
func printLines(r io.Reader, filter string) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		printEventLine(sc.Text(), filter)
	}
	return sc.Err()
}

// printLastN reads all lines and prints only the last n matching ones.
func printLastN(r io.Reader, filter string, n int) error {
	sc := bufio.NewScanner(r)
	var buf []string
	for sc.Scan() {
		line := sc.Text()
		if matchesFilter(line, filter) {
			buf = append(buf, line)
			if len(buf) > n {
				buf = buf[1:]
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, line := range buf {
		printEventLine(line, "")
	}
	return nil
}

// tailFile polls f for new lines, printing each as it arrives.
func tailFile(f *os.File, filter string) error {
	sc := bufio.NewScanner(f)
	for {
		for sc.Scan() {
			printEventLine(sc.Text(), filter)
		}
		if err := sc.Err(); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
		// Reset scanner to continue reading from current position.
		sc = bufio.NewScanner(f)
	}
}

// tailEvents waits for the log file to appear, then tails it.
func tailEvents(path, filter string) error {
	for {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				return err
			}
			return tailFile(f, filter)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// matchesFilter returns true if line contains an event whose type starts with filter.
func matchesFilter(line, filter string) bool {
	if filter == "" || line == "" {
		return line != ""
	}
	// Quick prefix check on the raw JSON before unmarshalling.
	return strings.Contains(line, `"`+filter)
}

// printEventLine decodes a NDJSON event line and prints it formatted.
func printEventLine(line, filter string) {
	if line == "" {
		return
	}
	var e event.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		// Corrupted line — print raw.
		fmt.Println(line)
		return
	}
	if filter != "" && !strings.HasPrefix(string(e.Type), filter) {
		return
	}
	ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
	if len(e.Data) == 0 {
		fmt.Printf("%s  %s\n", ts, e.Type)
		return
	}
	data, _ := json.Marshal(e.Data)
	fmt.Printf("%s  %-36s  %s\n", ts, e.Type, data)
}
