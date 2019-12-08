/*
 * This file is part of Crocodile Game Bot.
 * Copyright (C) 2019  Viktor
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"

	"gitlab.com/pviktor/crocodile-game-bot/crocodile"
	"gitlab.com/pviktor/crocodile-game-bot/model"
	"gitlab.com/pviktor/crocodile-game-bot/storage"
	"gitlab.com/pviktor/crocodile-game-bot/utils"
)

var machines map[int64]*crocodile.Machine
var fabric *crocodile.MachineFabric
var bot *tb.Bot

var wordsInlineKeys [][]tb.InlineButton
var newGameInlineKeys [][]tb.InlineButton
var ratingGetter RatingGetter

var DEBUG = false

type RatingGetter interface {
	GetRating(chatID int64) ([]model.UserInChat, error)
}

type dbCredentials struct {
	Host,
	User,
	Pass,
	Name string

	Port int
	KW   storage.KW
}

func loggerMiddlewarePoller(upd *tb.Update) bool {
	if upd.Message != nil && upd.Message.Chat != nil && upd.Message.Sender != nil {
		log.Printf(
			"Received update, chat: %d, chatTitle: \"%s\", user: %d",
			upd.Message.Chat.ID,
			upd.Message.Chat.Title,
			upd.Message.Sender.ID,
		)
	}
	return true
}

func getDbCredentialsFromEnv() (dbCredentials, error) {
	prefix := "CROCODILE_GAME_DB_"
	ret := dbCredentials{}
	ret.KW = storage.KW{}
	envList := os.Environ()
	env := make(map[string]string)
	for _, v := range envList {
		kv := strings.Split(v, "=")
		env[kv[0]] = kv[1]
	}
	var err error

	ret.Port, err = strconv.Atoi(env[prefix+"PORT"])
	if err != nil {
		return ret, err
	}
	delete(env, prefix+"PORT")

	ret.Host = env[prefix+"HOST"]
	ret.User = env[prefix+"USER"]
	ret.Pass = env[prefix+"PASS"]
	ret.Name = env[prefix+"NAME"]
	delete(env, prefix+"HOST")
	delete(env, prefix+"USER")
	delete(env, prefix+"PASS")
	delete(env, prefix+"NAME")

	for k, v := range env {
		if strings.HasPrefix(k, prefix) {
			ret.KW[strings.ToLower(strings.TrimPrefix(k, prefix))] = v
		}
	}

	return ret, nil
}

func main() {
	logInit()
	if os.Getenv("CROCODILE_GAME_DEV") != "" {
		DEBUG = true
		setLogLevel("TRACE")
	}

	log.Info("Loading words")
	f, err := os.Open("dictionaries/word_rus_min.txt")
	if err != nil {
		log.Fatalf("Cannot open dictionary: %v", err)
	}
	wordsProvider, _ := crocodile.NewWordsProviderReader(f)

	log.Info("Readind DB env variables")
	creds, err := getDbCredentialsFromEnv()
	if err != nil {
		log.Fatalf("Cannot get database credentials from ENV: %v", err)
	}

	log.Info("Connecting to the database")
	pg, err := storage.NewPostgres(storage.NewConnString(
		creds.Host, creds.User,
		creds.Pass, creds.Name,
		creds.Port, creds.KW,
	))
	if err != nil {
		log.Fatalf("Cannot connect to database (%s, %s) on host %s: %v", creds.User, creds.Name, creds.Host, err)
	}

	ratingGetter = pg

	log.Info("Creating games fabric")
	fabric = crocodile.NewMachineFabric(pg, wordsProvider, log)
	machines = make(map[int64]*crocodile.Machine)

	log.Info("Connecting to Telegram API")
	poller := &tb.LongPoller{Timeout: 15 * time.Second}
	settings := tb.Settings{
		Token:  os.Getenv("CROCODILE_GAME_BOT_TOKEN"),
		Poller: tb.NewMiddlewarePoller(poller, loggerMiddlewarePoller),
	}
	bot, err = tb.NewBot(settings)
	if err != nil {
		if err != nil {
			log.Fatalf("Cannot connect to Telegram API: %v", err)
		}
	}

	log.Info("Binding handlers")
	bot.Handle(tb.OnText, textHandler)
	bot.Handle("/start", startNewGameHandler)
	bot.Handle("/rating", ratingHandler)
	bot.Handle("/cancel", func(m *tb.Message) {})
	bindButtonsHandlers(bot)

	log.Info("Starting the bot")
	bot.Start()
}

func ratingHandler(m *tb.Message) {
	rating, err := ratingGetter.GetRating(m.Chat.ID)
	if err != nil {
		log.Errorf("ratingHandler: cannot get rating %v:", err)
	}
	if len(rating) < 1 {
		bot.Send(m.Chat, "Данных пока недостаточно!")
		return
	}

	outString := "Топ-25 <b>игроков в крокодила</b> 🐊\n\n"
	for k, v := range rating {
		outString += fmt.Sprintf(
			"<b>%d</b>. %s — %d %s.\n",
			k+1,
			html.EscapeString(v.Name),
			v.Guessed,
			utils.DetectCaseAnswers(v.Guessed),
		)
	}

	_, err = bot.Send(m.Chat, outString, tb.ModeHTML)
	if err != nil {
		log.Errorf("ratingHandler: cannot send rating: %v", err)
	}
}

func startNewGameHandler(m *tb.Message) {
	// if m.Private() {
	// 	bot.Send(m.Sender, "Начать игру можно только в общем чате.")
	// }

	// If machine for this chat has been created already
	if _, ok := machines[m.Chat.ID]; !ok {
		machine := fabric.NewMachine(m.Chat.ID, m.ID)
		machines[m.Chat.ID] = machine
	}

	username := strings.TrimSpace(m.Sender.FirstName + " " + m.Sender.LastName)

	_, err := machines[m.Chat.ID].StartNewGameAndReturnWord(m.Sender.ID, username)

	if err != nil {
		if err.Error() == crocodile.ErrGameAlreadyStarted {
			_, ms, _ := utils.CalculateTimeDiff(time.Now(), machines[m.Chat.ID].GetStartedTime())

			if ms < 2 {
				bot.Send(m.Chat, "Игра уже начата! Ожидайте 2 минуты")
				return
			} else {
				machines[m.Chat.ID].StopGame()
				_, err = machines[m.Chat.ID].StartNewGameAndReturnWord(m.Sender.ID, username)
				if err != nil {
					log.Println(err)
				}
			}
		} else {
			log.Println(err)
			return
		}
	}

	bot.Send(
		m.Chat,
		m.Sender.FirstName+" должен объяснить слово за 2 минуты",
		&tb.ReplyMarkup{InlineKeyboard: wordsInlineKeys},
	)
}

func startNewGameHandlerCallback(c *tb.Callback) {
	m := c.Message
	var ma *crocodile.Machine
	// if m.Private() {
	// 	bot.Send(m.Sender, "Начать игру можно только в общем чате.")
	// }

	// If machine for this chat has been created already
	if _, ok := machines[m.Chat.ID]; !ok {
		machine := fabric.NewMachine(m.Chat.ID, m.ID)
		machines[m.Chat.ID] = machine
	}

	ma = machines[m.Chat.ID]

	username := strings.TrimSpace(c.Sender.FirstName + " " + c.Sender.LastName)
	_, err := ma.StartNewGameAndReturnWord(c.Sender.ID, username)

	if err != nil {
		if err.Error() == crocodile.ErrGameAlreadyStarted {
			_, ms, _ := utils.CalculateTimeDiff(time.Now(), ma.GetStartedTime())

			if ms < 2 {
				bot.Respond(c, &tb.CallbackResponse{Text: "Игра уже начата! Ожидайте 2 минуты"})
				return
			} else {
				ma.StopGame()
				_, err = ma.StartNewGameAndReturnWord(c.Sender.ID, username)
				if err != nil {
					log.Println(err)
				}
			}
		} else if err.Error() == crocodile.ErrWaitingForWinnerRespond {
			bot.Respond(c, &tb.CallbackResponse{Text: "У победителя есть 5 секунд на решение!"})
			return
		} else {
			log.Println(err)
			return
		}
	}

	bot.Send(
		m.Chat,
		c.Sender.FirstName+" должен объяснить слово за 2 минуты",
		&tb.ReplyMarkup{InlineKeyboard: wordsInlineKeys},
	)
}

func textHandler(m *tb.Message) {
	if ma, ok := machines[m.Chat.ID]; ok {
		if ma.GetHost() != m.Sender.ID || DEBUG {
			word := strings.TrimSpace(strings.ToLower(m.Text))
			username := strings.TrimSpace(m.Sender.FirstName + " " + m.Sender.LastName)
			if ma.CheckWordAndSetWinner(word, m.Sender.ID, username) {
				username := strings.TrimSpace(m.Sender.FirstName + " " + m.Sender.LastName)
				bot.Send(
					m.Chat,
					fmt.Sprintf(
						"%s отгадал слово <b>%s</b>",
						username, word,
					),
					tb.ModeHTML,
					&tb.ReplyMarkup{InlineKeyboard: newGameInlineKeys},
				)
			}
		}
	}
}

func seeWordCallbackHandler(c *tb.Callback) {
	if m, ok := machines[c.Message.Chat.ID]; ok {
		var message string

		if c.Sender.ID != m.GetHost() {
			message = "Это слово предназначено не для тебя!"
		} else {
			message = m.GetWord()
		}

		bot.Respond(c, &tb.CallbackResponse{Text: message})
	}
}

func nextWordCallbackHandler(c *tb.Callback) {
	if m, ok := machines[c.Message.Chat.ID]; ok {
		var message string

		if c.Sender.ID != m.GetHost() {
			message = "Это слово предназначено не для тебя!"
		} else {
			message, _ = m.SetNewRandomWord()
		}

		bot.Respond(c, &tb.CallbackResponse{Text: message})
	}
}

func bindButtonsHandlers(bot *tb.Bot) {
	seeWord := tb.InlineButton{Unique: "see_word", Text: "Посмотреть слово"}
	nextWord := tb.InlineButton{Unique: "next_word", Text: "Следующее слово"}
	newGame := tb.InlineButton{Unique: "new_game", Text: "Хочу быть ведущим!"}

	wordsInlineKeys = [][]tb.InlineButton{[]tb.InlineButton{seeWord}, []tb.InlineButton{nextWord}}
	newGameInlineKeys = [][]tb.InlineButton{[]tb.InlineButton{newGame}}

	bot.Handle(&newGame, startNewGameHandlerCallback)
	bot.Handle(&seeWord, seeWordCallbackHandler)
	bot.Handle(&nextWord, nextWordCallbackHandler)
}