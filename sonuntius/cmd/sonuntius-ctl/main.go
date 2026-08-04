// Command sonuntius-ctl is a small CLI for poking the ma-bridge IPC
// socket from inside the container. Useful during development and as
// the smoke-test driver.
//
// Examples:
//
//	sonuntius-ctl play --provider ytmusic --track-id abc123
//	sonuntius-ctl play --provider url     --url https://example.com/a.mp3
//	sonuntius-ctl transport --command pause
//	sonuntius-ctl volume --level 0.4
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage: sonuntius-ctl <command> [flags]

Commands:
  play       send a PlayIntent
  transport  send a TransportCommand
  volume     send a VolumeCommand

Global flags:
  --socket   path to the IPC socket (default %s)
`, ipc.DefaultSocketPath)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var socket string

	switch cmd {
	case "play":
		fs := flag.NewFlagSet("play", flag.ExitOnError)
		provider := fs.String("provider", "", "ytmusic | tidal | url")
		trackID := fs.String("track-id", "", "track id (ytmusic/tidal)")
		url := fs.String("url", "", "media url (provider=url)")
		source := fs.String("source", "cli", "originating receiver name")
		fs.StringVar(&socket, "socket", "", "IPC socket path")
		_ = fs.Parse(args)
		if *provider == "" {
			fmt.Fprintln(os.Stderr, "play: --provider is required")
			os.Exit(2)
		}
		send(socket, &events.PlayIntent{
			Provider: *provider,
			TrackID:  *trackID,
			URL:      *url,
			Source:   *source,
		})
	case "transport":
		fs := flag.NewFlagSet("transport", flag.ExitOnError)
		command := fs.String("command", "", "play|pause|stop|next|previous|seek")
		position := fs.Float64("position", -1, "seek position in seconds")
		fs.StringVar(&socket, "socket", "", "IPC socket path")
		_ = fs.Parse(args)
		if *command == "" {
			fmt.Fprintln(os.Stderr, "transport: --command is required")
			os.Exit(2)
		}
		ev := &events.TransportCommand{Command: *command, Source: "cli"}
		if *position >= 0 {
			p := *position
			ev.Position = &p
		}
		send(socket, ev)
	case "volume":
		fs := flag.NewFlagSet("volume", flag.ExitOnError)
		level := fs.Float64("level", -1, "volume level 0.0–1.0")
		muted := fs.Bool("muted", false, "set mute state")
		setMute := fs.Bool("set-muted", false, "actually apply --muted")
		fs.StringVar(&socket, "socket", "", "IPC socket path")
		_ = fs.Parse(args)
		ev := &events.VolumeCommand{Source: "cli"}
		if *level >= 0 {
			l := *level
			ev.Level = &l
		}
		if *setMute {
			m := *muted
			ev.Muted = &m
		}
		send(socket, ev)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func send(socket string, ev events.Event) {
	cli, err := ipc.Dial(socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cli.Close()
	if err := cli.Send(ev); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("sent: %s %+v\n", ev.EventType(), ev)
}
