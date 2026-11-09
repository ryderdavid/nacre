package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/config"
)

const decisionsUsage = "usage: nacred decisions [REPO] | nacred decision ID"

// runDecisions prints what waits on Ryder, for the Needs you walk-through:
// `nacred decisions [REPO]` lists the pending and held decisions of a
// repository (a directory, name or id; the current directory by default),
// in nacred's queue order, and `nacred decision ID` prints one. Both only
// read, so a light chat turn may run them.
func runDecisions(cfg *config.Config, cmd string, args []string, out io.Writer) error {
	c := &roundClient{base: "http://" + cfg.ResolveListen("", 3*time.Second), token: clientToken(cfg)}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if cmd == "decision" {
		if len(args) != 1 {
			return errors.New(decisionsUsage)
		}
		var d json.RawMessage
		if err := c.do("GET", "/v1/decisions/"+url.PathEscape(args[0]), nil, &d); err != nil {
			return err
		}
		return enc.Encode(d)
	}
	ref := "."
	switch len(args) {
	case 0:
	case 1:
		ref = args[0]
	default:
		return errors.New(decisionsUsage)
	}
	repo, _, err := c.findRepo(ref)
	if err != nil {
		return err
	}
	var list struct {
		Decisions []map[string]any `json:"decisions"`
	}
	if err := c.do("GET", "/v1/decisions?repo="+url.QueryEscape(repo), nil, &list); err != nil {
		return err
	}
	waiting := []map[string]any{}
	for _, d := range list.Decisions {
		// What counts toward Needs you: pending, held, or a failed or unknown outcome not yet seen.
		st, _ := d["state"].(string)
		seen, _ := d["acknowledged"].(bool)
		if st == "pending" || st == "held" || ((st == "failed" || st == "unknown") && !seen) {
			waiting = append(waiting, d)
		}
	}
	return enc.Encode(map[string]any{"repo": repo, "waiting": waiting})
}
