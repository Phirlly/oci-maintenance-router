package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

const minimumPercent uint64 = 97

type coverage struct{ covered, total uint64 }

func readCoverage(input io.Reader) (coverage, error) {
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return coverage{}, errors.New("missing coverage mode")
	}
	switch scanner.Text() {
	case "mode: set", "mode: count", "mode: atomic":
	default:
		return coverage{}, errors.New("unsupported coverage mode")
	}
	blocks := map[string]block{}
	for scanner.Scan() {
		location, current, err := readBlock(scanner.Text())
		if err != nil {
			return coverage{}, err
		}
		if prior, exists := blocks[location]; exists {
			if prior.statements != current.statements {
				return coverage{}, errors.New("inconsistent duplicate statement count")
			}
			current.covered = current.covered || prior.covered
		}
		blocks[location] = current
	}
	if err := scanner.Err(); err != nil {
		return coverage{}, err
	}
	var result coverage
	for _, current := range blocks {
		if current.statements > maxStatements-result.total {
			return coverage{}, errors.New("statement count exceeds exact comparison bound")
		}
		result.total += current.statements
		if current.covered {
			result.covered += current.statements
		}
	}
	if result.total == 0 {
		return coverage{}, errors.New("profile has no statements")
	}
	return result, nil
}

// Multiplications cannot overflow for totals accepted by readCoverage.
func (c coverage) passes() bool {
	return c.total > 0 && c.covered*100 >= c.total*minimumPercent
}

const maxStatements = ^uint64(0) / 100

var recordPattern = regexp.MustCompile(`^(.+):([0-9]+)\.([0-9]+),([0-9]+)\.([0-9]+) ([0-9]+) ([0-9]+)$`)

type block struct {
	statements uint64
	covered    bool
}

func readBlock(line string) (string, block, error) {
	match := recordPattern.FindStringSubmatch(line)
	if match == nil {
		return "", block{}, errors.New("malformed coverage block")
	}
	var numbers [6]uint64
	for i, text := range match[2:] {
		value, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return "", block{}, errors.New("invalid coverage number")
		}
		numbers[i] = value
	}
	location := fmt.Sprintf("%s:%d.%d,%d.%d", match[1], numbers[0], numbers[1], numbers[2], numbers[3])
	return location, block{numbers[4], numbers[5] > 0}, nil
}
