package models

type AnalysisRequest struct {
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

type AnalysisResponse struct {
	Summary            string        `json:"summary"`
	SourceURL          string        `json:"source_url,omitempty"`
	FactCheck          FactCheck     `json:"fact_check"`
	Manipulations      []string      `json:"manipulations"`
	LogicalIssues      []string      `json:"logical_issues"`
	CredibilityScore   int           `json:"credibility_score"`
	ScoreBreakdown     string        `json:"score_breakdown,omitempty"`
	FinalVerdict       string        `json:"final_verdict,omitempty"`
	VerdictExplanation string        `json:"verdict_explanation,omitempty"`
	Reasoning          string        `json:"reasoning"`
	Sources            []Source      `json:"sources,omitempty"`
	Verification       Verification  `json:"verification,omitempty"`
	Usage              *TokenUsage   `json:"usage,omitempty"`
	RawResponse        string        `json:"raw_response,omitempty"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Verification struct {
	IsFake           bool     `json:"is_fake"`
	FakeReasons      []string `json:"fake_reasons,omitempty"`
	RealInformation  string   `json:"real_information,omitempty"`
	VerifiedSources  interface{} `json:"verified_sources,omitempty"` // Может быть []string или []Source
}

type Source struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type FactCheck struct {
	VerifiableFacts   []string `json:"verifiable_facts"`
	OpinionsAsFacts   []string `json:"opinions_as_facts"`
	MissingEvidence   []string `json:"missing_evidence"`
}

type ChatRequest struct {
	Message         string           `json:"message"`
	AnalysisContext *AnalysisResponse `json:"analysis_context,omitempty"`
}

type ChatResponse struct {
	Response string      `json:"response"`
	Usage    *TokenUsage `json:"usage,omitempty"`
}
