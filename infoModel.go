package main

// Shows metadata about the game. Stub for now.
type infoModel struct{}

func newInfoModel() infoModel {
	return infoModel{}
}

func (i infoModel) View(w, h int) string {
	return renderPanel("INFO", w, h, infoBg)
}
