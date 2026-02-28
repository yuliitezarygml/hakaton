package main

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func escHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func clamp(n, max int) int {
	if n > max {
		return max
	}
	return n
}

func FormatResult(r *AnalysisResult, sourceLabel string) string {
	score := r.CredibilityScore
	var emoji, label string
	switch {
	case score <= 3:
		emoji = "🔴"
		label = "ДЕЗИНФОРМАЦИЯ"
	case score <= 6:
		emoji = "🟡"
		label = "СОМНИТЕЛЬНО"
	default:
		emoji = "🟢"
		label = "ДОСТОВЕРНО"
	}

	var b strings.Builder

	// Source label (for forwarded messages)
	if sourceLabel != "" {
		b.WriteString(fmt.Sprintf("📢 <b>Источник:</b> %s\n", sourceLabel))
	}

	// Header
	b.WriteString(fmt.Sprintf("%s <b>%d/10 — %s</b>\n", emoji, score, label))

	// Score bar
	filled := score
	empty := 10 - score
	b.WriteString("<code>[")
	b.WriteString(strings.Repeat("█", filled))
	b.WriteString(strings.Repeat("░", empty))
	b.WriteString(fmt.Sprintf("]</code> %d/10\n", score))

	// Summary
	if r.Summary != "" {
		b.WriteString(fmt.Sprintf("\n📝 %s\n", escHTML(r.Summary)))
	}

	// Manipulations
	if len(r.Manipulations) > 0 {
		b.WriteString("\n⚠️ <b>Манипуляции:</b>\n")
		for _, m := range r.Manipulations[:clamp(len(r.Manipulations), 5)] {
			b.WriteString(fmt.Sprintf("• %s\n", escHTML(m)))
		}
	}

	// Logical issues
	if len(r.LogicalIssues) > 0 {
		b.WriteString("\n🔍 <b>Логические ошибки:</b>\n")
		for _, l := range r.LogicalIssues[:clamp(len(r.LogicalIssues), 5)] {
			b.WriteString(fmt.Sprintf("• %s\n", escHTML(l)))
		}
	}

	// Fact check
	if r.FactCheck != nil {
		if len(r.FactCheck.MissingEvidence) > 0 {
			b.WriteString("\n❓ <b>Без доказательств:</b>\n")
			for _, e := range r.FactCheck.MissingEvidence[:clamp(len(r.FactCheck.MissingEvidence), 3)] {
				b.WriteString(fmt.Sprintf("• %s\n", escHTML(e)))
			}
		}
		if len(r.FactCheck.OpinionsAsFacts) > 0 {
			b.WriteString("\n💬 <b>Мнения как факты:</b>\n")
			for _, o := range r.FactCheck.OpinionsAsFacts[:clamp(len(r.FactCheck.OpinionsAsFacts), 3)] {
				b.WriteString(fmt.Sprintf("• %s\n", escHTML(o)))
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func FormatProgress(events []string) string {
	if len(events) == 0 {
		return "⏳ <b>Анализирую...</b>"
	}
	last := events[len(events)-1]
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	sp := spinner[len(events)%len(spinner)]
	return fmt.Sprintf("%s <b>Анализирую...</b>\n\n<code>%s</code>", sp, escHTML(last))
}

func GetResultKeyboard(shareURL, reScanData string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	var row1 []tgbotapi.InlineKeyboardButton
	if shareURL != "" {
		row1 = append(row1, tgbotapi.NewInlineKeyboardButtonURL("🔗 Поделиться", shareURL))
	}
	if reScanData != "" {
		row1 = append(row1, tgbotapi.NewInlineKeyboardButtonData("🔄 Перепроверить", "rescan:"+reScanData))
	}

	if len(row1) > 0 {
		rows = append(rows, row1)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
