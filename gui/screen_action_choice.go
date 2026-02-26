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
	choices := []string{"BACKUP WALLET", "RECOVER DESCR."}
	if debugLoadTestWalletEnabled(ctx) {
		choices = append(choices, "LOAD TEST WALLET")
	}
	cs := &ChoiceScreen{
		Title:   "Action",
		Lead:    "Choose action",
		Choices: choices,
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
	case 2:
		if debugLoadTestWalletEnabled(ctx) {
			return newLoadTestWalletScreen(th)
		}
		return &MainMenuScreen{}
	default:
		return &MainMenuScreen{}
	}
}
