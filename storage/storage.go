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

package storage

import (
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"fmt"
	"gitlab.com/pviktor/crocodile-game-bot/model"
)

type Postgres struct {
	db    *gorm.DB
	mutex *sync.Mutex
}

type KW map[string]interface{}

func NewConnString(host, user, pass, dbname string, port int, kw KW) string {
	baseString := fmt.Sprintf(
		"host=%s port=%d user=%s dbname=%s password=%s",
		host, port, user, dbname, pass,
	)

	for k, v := range kw {
		baseString += fmt.Sprintf(" %s=%v", k, v)
	}

	return baseString
}

func NewPostgres(conn string) (*Postgres, error) {
	db, err := gorm.Open("postgres", conn)

	if err != nil {
		return nil, err
	}

	return &Postgres{
		db:    db,
		mutex: &sync.Mutex{},
	}, nil
}

func (p *Postgres) IncrementUserStats(givenUser model.UserInChat) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var user model.UserInChat

	p.db.FirstOrCreate(&user, model.UserInChat{
		ID:     givenUser.ID,
		ChatID: givenUser.ChatID,
	})

	p.db.Table("user_in_chats").
		Where("id = ? AND chat_id = ?", givenUser.ID, givenUser.ChatID).
		Updates(map[string]interface{}{
			"name":     givenUser.Name,
			"was_host": user.WasHost + givenUser.WasHost,
			"success":  user.Success + givenUser.Success,
			"guessed":  user.Guessed + givenUser.Guessed,
		})

	return nil
}

func (p *Postgres) GetRating(chatID int64) ([]model.UserInChat, error) {
	var users []model.UserInChat
	p.db.Where("guessed > 0 AND chat_id = ?", chatID).Limit(25).Order("guessed desc").Find(&users)
	return users, nil
}
