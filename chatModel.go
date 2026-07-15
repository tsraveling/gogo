package main

// chatModel shows the game chat log and (online) a text entry. Stub for now.
type chatModel struct{}

func newChatModel() chatModel {
	return chatModel{}
}

func (c chatModel) View(w, h int) string {
	return renderPanel("CHAT", w, h, chatBg)
}
