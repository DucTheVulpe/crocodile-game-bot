package handler

import (
	tele "gopkg.in/tucnak/telebot.v3"
	"gopkg.in/tucnak/telebot.v3/layout"
)

func New(h Handler) handler {
	return handler{
		b:   h.Bot,
		lt:  h.Layout,
	}
}

type (
	Handler struct {
		Layout *layout.Layout
		Bot    *tele.Bot
	}

	handler struct {
		bot   *tele.Bot
		lt  *layout.Layout
	}
)
