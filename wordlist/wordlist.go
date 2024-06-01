package wordlist

import (
	"io/ioutil"
	"math/rand"
	"strings"
	"time"
)

var words []string

func Initialize() error {
	data, err := ioutil.ReadFile("dictionaries/az.txt")
	if err != nil {
		return err
	}
	words = strings.Fields(string(data))
	return nil
}

func Get(index int) string {
	return words[index]
}

func Rand() int {
	return rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(words))
}

func Prev(index int) int {
	if index != 0 {
		index--
	}
	return index
}

func Next(index int) int {
	if len(words) > index+1 {
		index++
	}
	return index
}
