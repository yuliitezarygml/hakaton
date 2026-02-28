// Основной JavaScript для функциональности сайта

document.addEventListener('DOMContentLoaded', function() {
    console.log('Fact Guard приложение загружено');
    initializeApp();
});

function initializeApp() {
    // Инициализация навигации
    setupNavigation();
    
    // Инициализация слайдера
    setupSlider();

    // Инициализация стриминга API
    setupApiStreaming();
}

function setupNavigation() {
    // Добавить активный класс к текущей странице
    const currentPath = window.location.pathname;
    const navLinks = document.querySelectorAll('.nav-menu a');
    
    navLinks.forEach(link => {
        if (link.getAttribute('href') === currentPath) {
            link.classList.add('active');
        }
    });

    // Обработчик клика на логотип для перехода на главную
    const logoImg = document.querySelector('.img-logo');
    const logoText = document.querySelector('.text-logo');
    
    if (logoImg) {
        logoImg.addEventListener('click', () => window.location.href = '/');
    }
    
    if (logoText) {
        logoText.addEventListener('click', () => window.location.href = '/');
    }
}

function setupSlider() {
    const cards = document.querySelectorAll('.card');
    const indicators = document.querySelectorAll('.indicator');
    const prevBtn = document.getElementById('prevBtn');
    const nextBtn = document.getElementById('nextBtn');
    let currentIndex = 0;
    const cardWidth = 320;
    const gap = 20;

    if (!cards.length) {
        return;
    }

    function updateSlider() {
        cards.forEach((card, index) => {
            let diff = index - currentIndex;
            // Wrap around
            if (diff > 1) diff -= 3;
            if (diff < -1) diff += 3;
            let offset = diff * (cardWidth + gap);
            card.style.transform = `translateX(calc(-50% + ${offset}px))`;
            card.classList.toggle('active', diff === 0);
        });
        indicators.forEach((indicator, index) => {
            indicator.classList.toggle('active', index === currentIndex);
        });
    }

    if (prevBtn) {
        prevBtn.addEventListener('click', () => {
            currentIndex = (currentIndex - 1 + cards.length) % cards.length;
            updateSlider();
        });
    }

    if (nextBtn) {
        nextBtn.addEventListener('click', () => {
            currentIndex = (currentIndex + 1) % cards.length;
            updateSlider();
        });
    }

    // Инициализация
    updateSlider();
}

function setArticleModalOpen(isOpen) {
    const modal = document.getElementById('articleModal');
    if (!modal) {
        return;
    }

    modal.classList.toggle('is-open', isOpen);
    modal.setAttribute('aria-hidden', isOpen ? 'false' : 'true');
    document.body.classList.toggle('modal-open', isOpen);

    if (!isOpen) {
        const articleContent = document.getElementById('articleContent');
        if (articleContent) {
            articleContent.innerHTML = '';
        }
    }
}

function openArticle(articleId) {
    const modal = document.getElementById('articleModal');
    const articleContent = document.getElementById('articleContent');
    
    if (modal && articleContent) {
        articleContent.innerHTML = '<p>Загрузка...</p>';
        setArticleModalOpen(true);

        // Получить статью с сервера
        fetch(`/article/${articleId}`, {
            headers: {
                'Accept': 'text/html'
            }
        })
        .then(response => {
            if (!response.ok) {
                throw new Error('Ошибка загрузки статьи');
            }
            return response.text();
        })
        .then(html => {
            articleContent.innerHTML = html;
        })
        .catch(error => {
            console.error('Ошибка при загрузке статьи:', error);
            articleContent.innerHTML = '<p>Ошибка при загрузке статьи. Попробуйте позже.</p>';
        });
    }
}

// Закрытие модального окна
function closeModal() {
    setArticleModalOpen(false);
}

// Обработчик клика на кнопку "Читать далее" и закрытие модала
document.addEventListener('click', function(event) {
    const readMore = event.target.closest('.read-more');
    if (readMore) {
        event.preventDefault();
        const articleId = readMore.getAttribute('data-article-id');
        if (articleId) {
            openArticle(articleId);
        }
        return;
    }

    const closeTrigger = event.target.closest('[data-modal-close="true"]');
    if (closeTrigger) {
        event.preventDefault();
        closeModal();
        return;
    }

    // Закрытие модала при клике на backdrop через JS
    const modal = document.getElementById('articleModal');
    if (modal && event.target === modal) {
        closeModal();
    }
});

// Закрытие модала по нажатию на Esc
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closeModal();
    }
});

function setupApiStreaming() {
    const responseBlock = document.querySelector('.api-response-block[data-api-url]');
    if (!responseBlock) {
        return;
    }

    const fakeResultRaw = responseBlock.getAttribute('data-fake-result');
    const fakeLogsRaw = responseBlock.getAttribute('data-fake-logs');
    const useFakeAttr = responseBlock.getAttribute('data-use-fake');
    const useFake = useFakeAttr === 'true';

    // DEBUG: показать значение флага в консоли
    console.log('[Fact Guard] data-use-fake attr:', useFakeAttr, '| useFake:', useFake);

    const apiUrl = responseBlock.getAttribute('data-api-url');

    const streamBase = responseBlock.getAttribute('data-stream-base') || '/api/analyze/stream';
    const streamUrl = `${streamBase}?url=${encodeURIComponent(apiUrl)}`;
    const logBody = document.getElementById('api-log-body');
    const logPlaceholder = document.getElementById('api-log-placeholder');

    // Обновление однострочного блока лога
    const updateSingleLogLine = (type, message) => {
        const singleLog = document.querySelector('.single-log-line');
        if (!singleLog) return;

        const dot = singleLog.querySelector('.log-dot');
        const badge = singleLog.querySelector('.log-type-badge');
        const msg = singleLog.querySelector('.log-message');
        const spinner = singleLog.querySelector('.log-spinner');

        // Удаляем все классы типов
        const types = ['start', 'progress', 'result', 'done', 'error'];
        if (dot) {
            types.forEach(t => dot.classList.remove(t));
            dot.classList.add(type);
        }
        if (badge) {
            types.forEach(t => badge.classList.remove(t));
            badge.classList.add(type);
            badge.textContent = type;
        }
        if (msg) {
            msg.textContent = message || '-';
        }
        // Скрываем спиннер при завершении или ошибке
        if (spinner) {
            spinner.style.display = (type === 'done' || type === 'error') ? 'none' : 'block';
        }
    };

    // Установка поля с текстом
    const setField = (key, value) => {
        const field = document.querySelector(`[data-field="${key}"]`);
        if (!field) {
            return;
        }
        field.textContent = value === null || value === undefined || value === '' ? '-' : value;
    };

    // Установка списка
    const setList = (key, items) => {
        const list = document.querySelector(`[data-field-list="${key}"]`);
        if (!list) {
            return;
        }
        list.innerHTML = '';
        if (!Array.isArray(items) || items.length === 0) {
            const li = document.createElement('li');
            li.className = 'placeholder-item';
            li.textContent = 'Нет данных';
            list.appendChild(li);
            return;
        }
        items.forEach(item => {
            const li = document.createElement('li');
            li.textContent = item;
            list.appendChild(li);
        });
        
        // Обновляем счётчик
        const countField = document.querySelector(`[data-field="${key}_count"]`);
        if (countField) {
            countField.textContent = items.length;
        }
    };

    // Установка источников (новый формат)
    const setSources = (sources) => {
        const list = document.querySelector('[data-field-sources]');
        if (!list) {
            return;
        }
        list.innerHTML = '';
        if (!Array.isArray(sources) || sources.length === 0) {
            const item = document.createElement('div');
            item.className = 'source-item placeholder-item';
            item.innerHTML = '<span>Нет данных об источниках</span>';
            list.appendChild(item);
            return;
        }
        sources.forEach(source => {
            const item = document.createElement('div');
            item.className = 'source-item';
            
            const title = document.createElement('div');
            title.className = 'source-item-title';
            title.textContent = source.title || 'Без названия';
            
            const url = document.createElement('a');
            url.className = 'source-item-url';
            url.href = source.url || '#';
            url.target = '_blank';
            url.textContent = source.url || '-';
            
            const desc = document.createElement('div');
            desc.className = 'source-item-desc';
            desc.textContent = source.description || '';
            
            item.appendChild(title);
            item.appendChild(url);
            if (source.description) {
                item.appendChild(desc);
            }
            list.appendChild(item);
        });
    };

    // Установка уровня вердикта (цветовая схема)
    const setVerdictLevel = (score) => {
        const verdictCard = document.querySelector('.verdict-card');
        if (!verdictCard) return;
        
        let level = 'pending';
        if (score >= 7) {
            level = 'high';
        } else if (score >= 4) {
            level = 'medium';
        } else if (score >= 0) {
            level = 'low';
        }
        verdictCard.setAttribute('data-verdict-level', level);
    };

    // Установка прогресс-бара
    const setScoreBar = (score) => {
        const bar = document.querySelector('[data-field="score_bar"]');
        if (bar) {
            const percentage = Math.max(0, Math.min(100, (score / 10) * 100));
            bar.style.width = `${percentage}%`;
        }
    };

    // Применение результата анализа
    const applyResult = (result) => {
        if (!result || typeof result !== 'object') {
            return;
        }
        
        // Основные поля
        const score = result.credibility_score || result.score || 0;
        setField('credibility_score', score);
        setScoreBar(score);
        setVerdictLevel(score);
        
        setField('final_verdict', result.final_verdict || result.verdict || '-');
        setField('verdict_explanation', result.verdict_explanation || '-');
        setField('summary', result.summary || '-');
        
        // Ссылка на источник
        const sourceLink = document.querySelector('[data-field="source_url_link"]');
        if (sourceLink && result.source_url) {
            sourceLink.href = result.source_url;
            sourceLink.textContent = result.source_url.length > 50 
                ? result.source_url.substring(0, 50) + '...' 
                : result.source_url;
        }
        
        // Списки
        const factCheck = result.fact_check || {};
        setList('verifiable_facts', factCheck.verifiable_facts);
        setList('opinions_as_facts', factCheck.opinions_as_facts);
        setList('missing_evidence', factCheck.missing_evidence);
        setList('manipulations', result.manipulations);
        setList('logical_issues', result.logical_issues);
        
        // Источники
        setSources(result.sources);
        
        // Обновляем статус лога
        const logStatus = document.querySelector('[data-field="log_status"]');
        if (logStatus) {
            logStatus.textContent = 'Завершено';
            logStatus.classList.add('done');
        }
    };

    // Добавление записи в лог (новый формат)
    const appendLog = (type, message) => {
        // Обновляем однострочный блок лога
        updateSingleLogLine(type, message);

        if (!logBody) {
            return;
        }
        if (logPlaceholder) {
            logPlaceholder.remove();
        }

        const entry = document.createElement('div');
        entry.className = `log-entry log-entry--${type}`;
        
        const dot = document.createElement('div');
        dot.className = 'log-dot';
        
        const content = document.createElement('div');
        content.className = 'log-content';
        
        const badge = document.createElement('span');
        badge.className = 'log-type-badge';
        badge.textContent = type;
        
        const msg = document.createElement('span');
        msg.className = 'log-message';
        msg.textContent = message || '-';
        
        content.appendChild(badge);
        content.appendChild(msg);

        // Добавляем кнопку копирования для result-типа
        if (type === 'result') {
            const copyBtn = document.createElement('button');
            copyBtn.className = 'log-copy-btn';
            copyBtn.innerHTML = '📋';
            copyBtn.title = 'Копировать';
            copyBtn.addEventListener('click', () => {
                navigator.clipboard.writeText(message).then(() => {
                    copyBtn.innerHTML = '✓';
                    copyBtn.classList.add('copied');
                    setTimeout(() => {
                        copyBtn.innerHTML = '📋';
                        copyBtn.classList.remove('copied');
                    }, 1500);
                });
            });
            content.appendChild(copyBtn);
        }

        entry.appendChild(dot);
        entry.appendChild(content);
        logBody.appendChild(entry);
        
        // Прокрутка к последнему сообщению
        logBody.scrollTop = logBody.scrollHeight;
    };

    const applyFakeLogs = (logs) => {
        if (!Array.isArray(logs) || logs.length === 0) {
            appendLog('done', '✅ Данные загружены из фейкового ответа.');
            return;
        }
        logs.forEach(log => {
            appendLog(log.type || 'progress', log.message);
        });
    };

    // Если fake-режим — используем только локальные данные, никаких запросов к серверу
    if (useFake) {
        if (fakeResultRaw) {
            try {
                const parsed = JSON.parse(fakeResultRaw);
                applyResult(parsed);
            } catch (error) {
                console.warn('Unable to parse fake result payload:', error);
            }
        }

        if (fakeLogsRaw) {
            try {
                const parsedLogs = JSON.parse(fakeLogsRaw);
                applyFakeLogs(parsedLogs);
            } catch (error) {
                console.warn('Unable to parse fake logs payload:', error);
                applyFakeLogs([]);
            }
        } else {
            applyFakeLogs([]);
        }

        // ВАЖНО: при fake-режиме НИКОГДА не создаём EventSource
        return;
    }

    if (!apiUrl) {
        return;
    }

    const source = new EventSource(streamUrl);

    source.addEventListener('start', (event) => {
        appendLog('start', event.data);
    });

    source.addEventListener('progress', (event) => {
        appendLog('progress', event.data);
    });

    source.addEventListener('result', (event) => {
        appendLog('result', event.data);
        try {
            const parsed = JSON.parse(event.data);
            applyResult(parsed);
        } catch (error) {
            console.warn('Unable to parse result payload:', error);
        }
    });

    source.addEventListener('done', (event) => {
        appendLog('done', event.data);
        source.close();
    });

    source.addEventListener('error', (event) => {
        const message = event && event.data ? event.data : 'Streaming connection error.';
        appendLog('error', message);
    });

    source.onmessage = (event) => {
        appendLog('progress', event.data);
    };
}

