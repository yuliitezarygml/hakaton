package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type GoogleFactCheckClient struct {
	APIKey string
}

func NewGoogleFactCheckClient(apiKey string) *GoogleFactCheckClient {
	return &GoogleFactCheckClient{APIKey: apiKey}
}

type GoogleFactCheckResponse struct {
	Claims []struct {
		Text        string `json:"text"`
		Claimant    string `json:"claimant"`
		ClaimDate   string `json:"claimDate"`
		ClaimReview []struct {
			Publisher struct {
				Name string `json:"name"`
				Site string `json:"site"`
			} `json:"publisher"`
			Url           string `json:"url"`
			Title         string `json:"title"`
			ReviewDate    string `json:"reviewDate"`
			TextualRating string `json:"textualRating"`
			LanguageCode  string `json:"languageCode"`
		} `json:"claimReview"`
	} `json:"claims"`
}

// Search queries the Google Fact Check Tools API
// We filter strictly with parameters to find misinformation relevant to Moldova
func (c *GoogleFactCheckClient) Search(query string) (string, error) {
	if c.APIKey == "" {
		return "", nil
	}

	log.Printf("[FACT CHECK] 🔍 Проверяю факты через Google Fact Check: %s", query)

	encodedQuery := url.QueryEscape(query)
	// languageCode=ro and languageCode=ru are most common for Moldova, but we leave it open to catch translation
	apiURL := fmt.Sprintf("https://factchecktools.googleapis.com/v1alpha1/claims:search?query=%s&key=%s", encodedQuery, c.APIKey)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[FACT CHECK] ❌ Ошибка сети: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[FACT CHECK] ❌ API вернуло статус: %d", resp.StatusCode)
		return "", fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var factCheckResp GoogleFactCheckResponse
	if err := json.Unmarshal(body, &factCheckResp); err != nil {
		log.Printf("[FACT CHECK] ❌ Ошибка парсинга JSON: %v", err)
		return "", err
	}

	if len(factCheckResp.Claims) == 0 {
		return "", nil
	}

	// Формируем результат в виде текста, который скормим AI
	var result string
	result += "\n\n--- 🕵️ БАЗА ПРОВЕРКИ ФАКТОВ (Google Fact Check Tools) ---\n"
	result += "ВАЖНО: Ниже приведены официальные проверки фактов, найденные независимыми журналистами. Если текущий текст совпадает с этими фейками, используйте это в анализе!\n"

	count := 0
	for _, claim := range factCheckResp.Claims {
		if count >= 3 {
			break // Берём только 3 самых релевантных фейка, чтобы не перегружать контекст
		}
		if len(claim.ClaimReview) > 0 {
			review := claim.ClaimReview[0]
			result += fmt.Sprintf("\n🔴 Утверждение: \"%s\"\n", claim.Text)
			result += fmt.Sprintf("📝 Вердикт журналистов: %s\n", review.TextualRating)
			result += fmt.Sprintf("📰 Источник: %s (%s)\n", review.Publisher.Name, review.Url)
			count++
		}
	}

	return result, nil
}
