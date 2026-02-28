"""
Конфигурация приложения Fact Guard
"""
import os
from dotenv import load_dotenv

# Загрузка переменных из .env файла
load_dotenv()

class Config:
    # Основные настройки приложения
    APP_NAME = "Fact Guard"
    VERSION = "1.0.0"
    
    # Настройки шаблонов
    TEMPLATES_DIR = "templates"
    STATIC_DIR = "static"
    
    # Параметры сервера
    DEBUG = os.getenv("DEBUG", "false").lower()
    HOST = "127.0.0.1"
    PORT = 8000

    # Использовать фейковые данные вместо обращения к API
    USE_FAKE_API_DATA = os.getenv("USE_FAKE_API_DATA", "false").strip().lower() == "true"
    