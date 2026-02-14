package gui

import "seedetcher.com/gui/op"

// ActionChoiceScreen selects the post-gate flow (backup or recovery).
type ActionChoiceScreen struct {
	Theme *Colors
}

func (s *ActionChoiceScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &singleTheme
	}
	cs := &ChoiceScreen{
		Title:   "Action",
		Lead:    "Choose action",
		Choices: []string{"BACKUP WALLET", "RECOVER DESCR."},
	}
	choice, ok := cs.Choose(ctx, ops, th)
	if !ok {
		return &MainMenuScreen{}
	}
	switch choice {
	case 0:
		return &BackupFlowScreen{Theme: th}
	case 1:
		return &RecoverDescriptorFlowScreen{Theme: th}
	default:
		return &MainMenuScreen{}
	}
}
