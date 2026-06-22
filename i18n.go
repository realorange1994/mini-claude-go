package main

import (
	"os"
	"strings"
)

// ─── i18n Multi-Language TUI (MiMo-Code 2) ─────────────────────────────────
//
// Internationalization for the TUI interface.
// Detects system locale and loads translation dictionaries.
//
// MiMo-Code source: cli/i18n.ts (15 lines)

// Locale represents a supported locale.
type Locale string

const (
	LocaleEN Locale = "en"
	LocaleZH Locale = "zh"
	LocaleJA Locale = "ja"
	LocaleFR Locale = "fr"
	LocaleRU Locale = "ru"
)

// I18n manages internationalization.
type I18n struct {
	locale      Locale
	translations map[Locale]map[string]string
}

// NewI18n creates a new i18n manager with auto-detected locale.
func NewI18n() *I18n {
	i := &I18n{
		translations: make(map[Locale]map[string]string),
	}
	i.registerDefaults()
	i.locale = i.detectLocale()
	return i
}

// detectLocale detects the system locale.
func (i *I18n) detectLocale() Locale {
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = os.Getenv("LC_ALL")
	}
	if lang == "" {
		lang = os.Getenv("LC_MESSAGES")
	}

	lang = strings.ToLower(lang)

	if strings.HasPrefix(lang, "zh") {
		return LocaleZH
	}
	if strings.HasPrefix(lang, "ja") {
		return LocaleJA
	}
	if strings.HasPrefix(lang, "fr") {
		return LocaleFR
	}
	if strings.HasPrefix(lang, "ru") {
		return LocaleRU
	}

	return LocaleEN
}

// registerDefaults registers default translations.
func (i *I18n) registerDefaults() {
	i.translations[LocaleEN] = map[string]string{
		"welcome":      "Welcome to MiMo-Code",
		"goodbye":      "Goodbye!",
		"error":        "Error",
		"success":      "Success",
		"loading":      "Loading...",
		"thinking":     "Thinking...",
		"tool_use":     "Using tool",
		"completed":    "Completed",
		"failed":       "Failed",
		"cancelled":    "Cancelled",
		"yes":          "Yes",
		"no":           "No",
		"ok":           "OK",
		"cancel":       "Cancel",
		"confirm":      "Confirm",
		"help":         "Help",
		"settings":     "Settings",
		"exit":         "Exit",
	}

	i.translations[LocaleZH] = map[string]string{
		"welcome":      "欢迎使用 MiMo-Code",
		"goodbye":      "再见！",
		"error":        "错误",
		"success":      "成功",
		"loading":      "加载中...",
		"thinking":     "思考中...",
		"tool_use":     "使用工具",
		"completed":    "已完成",
		"failed":       "失败",
		"cancelled":    "已取消",
		"yes":          "是",
		"no":           "否",
		"ok":           "确定",
		"cancel":       "取消",
		"confirm":      "确认",
		"help":         "帮助",
		"settings":     "设置",
		"exit":         "退出",
	}

	i.translations[LocaleJA] = map[string]string{
		"welcome":      "MiMo-Codeへようこそ",
		"goodbye":      "さようなら！",
		"error":        "エラー",
		"success":      "成功",
		"loading":      "読み込み中...",
		"thinking":     "考え中...",
		"tool_use":     "ツール使用",
		"completed":    "完了",
		"failed":       "失敗",
		"cancelled":    "キャンセル済み",
		"yes":          "はい",
		"no":           "いいえ",
		"ok":           "OK",
		"cancel":       "キャンセル",
		"confirm":      "確認",
		"help":         "ヘルプ",
		"settings":     "設定",
		"exit":         "終了",
	}

	i.translations[LocaleFR] = map[string]string{
		"welcome":      "Bienvenue dans MiMo-Code",
		"goodbye":      "Au revoir !",
		"error":        "Erreur",
		"success":      "Succès",
		"loading":      "Chargement...",
		"thinking":     "Réflexion...",
		"tool_use":     "Utilisation de l'outil",
		"completed":    "Terminé",
		"failed":       "Échoué",
		"cancelled":    "Annulé",
		"yes":          "Oui",
		"no":           "Non",
		"ok":           "OK",
		"cancel":       "Annuler",
		"confirm":      "Confirmer",
		"help":         "Aide",
		"settings":     "Paramètres",
		"exit":         "Quitter",
	}

	i.translations[LocaleRU] = map[string]string{
		"welcome":      "Добро пожаловать в MiMo-Code",
		"goodbye":      "До свидания!",
		"error":        "Ошибка",
		"success":      "Успех",
		"loading":      "Загрузка...",
		"thinking":     "Думает...",
		"tool_use":     "Использование инструмента",
		"completed":    "Завершено",
		"failed":       "Ошибка",
		"cancelled":    "Отменено",
		"yes":          "Да",
		"no":           "Нет",
		"ok":           "OK",
		"cancel":       "Отмена",
		"confirm":      "Подтвердить",
		"help":         "Помощь",
		"settings":     "Настройки",
		"exit":         "Выход",
	}
}

// T translates a key to the current locale.
func (i *I18n) T(key string) string {
	if translations, ok := i.translations[i.locale]; ok {
		if val, ok := translations[key]; ok {
			return val
		}
	}

	// Fallback to English
	if translations, ok := i.translations[LocaleEN]; ok {
		if val, ok := translations[key]; ok {
			return val
		}
	}

	return key
}

// SetLocale sets the locale.
func (i *I18n) SetLocale(locale Locale) {
	i.locale = locale
}

// GetLocale returns the current locale.
func (i *I18n) GetLocale() Locale {
	return i.locale
}

// FormatLocaleInfo formats locale info for display.
func FormatLocaleInfo(i18n *I18n) string {
	return "Locale: " + string(i18n.GetLocale())
}
