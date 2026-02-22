package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

type PromptConfig struct {
	SystemPrompt     SystemPrompt      `json:"system_prompt"`
	Examples         Examples          `json:"examples"`
	CredibilityScale map[string]string `json:"credibility_scale"`
}

type SystemPrompt struct {
	Role              string            `json:"role"`
	Task              string            `json:"task"`
	ScoringRules      string            `json:"scoring_rules"`
	AnalysisAlgorithm []AnalysisStep    `json:"analysis_algorithm"`
	Tone              string            `json:"tone"`
	OutputFormat      OutputFormat      `json:"output_format"`
}

type AnalysisStep struct {
	Step        int    `json:"step"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type OutputFormat struct {
	Type      string                 `json:"type"`
	Structure map[string]interface{} `json:"structure"`
}

type Examples struct {
	ScoreCalibration  map[string]string `json:"score_calibration"`
	ManipulationTypes []string          `json:"manipulation_types"`
	LogicalFallacies  []string          `json:"logical_fallacies"`
}

func LoadPromptConfig(path string) (*PromptConfig, error) {
	log.Printf("[PROMPT] Загружаю конфигурацию промпта из: %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}

	var config PromptConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	log.Printf("[PROMPT] ✓ Конфигурация загружена успешно")
	log.Printf("[PROMPT]   - Шагов анализа: %d", len(config.SystemPrompt.AnalysisAlgorithm))
	log.Printf("[PROMPT]   - Типов манипуляций: %d", len(config.Examples.ManipulationTypes))

	return &config, nil
}

func (pc *PromptConfig) BuildSystemPrompt() string {
	var b strings.Builder

	// Роль
	b.WriteString(pc.SystemPrompt.Role)
	b.WriteString("\n\n")

	// Задача
	b.WriteString(pc.SystemPrompt.Task)
	b.WriteString("\n\n")

	// Правила оценки (новое — ключевое для калибровки)
	if pc.SystemPrompt.ScoringRules != "" {
		b.WriteString(pc.SystemPrompt.ScoringRules)
		b.WriteString("\n\n")
	}

	// Калибровка шкалы с примерами
	if len(pc.Examples.ScoreCalibration) > 0 {
		b.WriteString("КАЛИБРОВКА ШКАЛЫ ОЦЕНОК:\n")
		for score, desc := range pc.Examples.ScoreCalibration {
			b.WriteString(fmt.Sprintf("  %s: %s\n", score, desc))
		}
		b.WriteString("\n")
	}

	// Алгоритм анализа
	b.WriteString("Алгоритм анализа:\n")
	for _, step := range pc.SystemPrompt.AnalysisAlgorithm {
		b.WriteString(fmt.Sprintf("%d. %s: %s\n", step.Step, step.Name, step.Description))
	}
	b.WriteString("\n")

	// Тон
	b.WriteString(pc.SystemPrompt.Tone)
	b.WriteString("\n\n")

	// Формат ответа
	b.WriteString(fmt.Sprintf("Ответь ТОЛЬКО в формате %s, без markdown, без пояснений до или после JSON:\n", pc.SystemPrompt.OutputFormat.Type))

	structureJSON, _ := json.MarshalIndent(pc.SystemPrompt.OutputFormat.Structure, "", "  ")
	b.WriteString(string(structureJSON))

	return b.String()
}

func (pc *PromptConfig) GetManipulationExamples() string {
	return "Примеры манипуляций: " + strings.Join(pc.Examples.ManipulationTypes, ", ")
}

func (pc *PromptConfig) GetLogicalFallacyExamples() string {
	return "Примеры логических ошибок: " + strings.Join(pc.Examples.LogicalFallacies, ", ")
}
