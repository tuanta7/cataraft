package query

import (
	"errors"
	"fmt"
	"strings"
)

var ErrUnsupportedCommand = errors.New("unsupported command")

type CommandType string

const (
	CommandGet CommandType = "GET"
	CommandSet CommandType = "SET"
)

type Command struct {
	Type  CommandType
	Key   string
	Value string
}

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(input string) (Command, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return Command{}, errors.New("query is required")
	}

	switch strings.ToUpper(fields[0]) {
	case string(CommandGet):
		if len(fields) != 2 {
			return Command{}, errors.New("GET requires exactly 1 argument")
		}
		return Command{Type: CommandGet, Key: fields[1]}, nil
	case string(CommandSet):
		if len(fields) < 3 {
			return Command{}, errors.New("SET requires a key and value")
		}
		return Command{
			Type:  CommandSet,
			Key:   fields[1],
			Value: strings.Join(fields[2:], " "),
		}, nil
	default:
		return Command{}, fmt.Errorf("%w: %s", ErrUnsupportedCommand, fields[0])
	}
}
