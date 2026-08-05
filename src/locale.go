// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

var supportedLanguages = []string{"de", "en"}

func normalizeLanguageSetting(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "auto", "system", "windows", "":
		return "auto"
	case "de", "de-de", "de_at", "de-at", "german", "deutsch":
		return "de"
	case "en", "en-us", "en-gb", "english":
		return "en"
	default:
		return "auto"
	}
}

func resolvedLanguage(cfg Config) string {
	configured := normalizeLanguageSetting(cfg.Language)
	if configured == "de" || configured == "en" {
		return configured
	}
	detected := strings.ToLower(strings.TrimSpace(detectSystemLanguage()))
	if strings.HasPrefix(detected, "de") {
		return "de"
	}
	return "en"
}

func responseLanguage(cfg Config) string {
	preferred := strings.ToLower(strings.TrimSpace(cfg.PreferredLanguage))
	switch preferred {
	case "de", "deutsch", "german", "de-de":
		return "Deutsch"
	case "en", "english", "englisch", "en-us", "en-gb":
		return "English"
	case "", "auto", "system", "windows":
		if resolvedLanguage(cfg) == "de" {
			return "Deutsch"
		}
		return "English"
	default:
		return cfg.PreferredLanguage
	}
}

func localized(lang, de, en string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return en
	}
	return de
}

func localizeConfigText(cfg Config, de, en string) string {
	return localized(resolvedLanguage(cfg), de, en)
}

func localizedToolInstallHint(cfg Config, text string) string {
	if resolvedLanguage(cfg) != "en" {
		return text
	}
	translations := map[string]string{
		"Android SDK Platform-Tools installieren.":                        "Install Android SDK Platform-Tools.",
		"Android SDK Command-Line Tools installieren.":                    "Install Android SDK Command-Line Tools.",
		"Android Emulator über den SDK Manager installieren.":             "Install Android Emulator through SDK Manager.",
		"Android SDK Build-Tools installieren.":                           "Install Android SDK Build-Tools.",
		"Bevorzugt den Gradle Wrapper des Projekts verwenden.":            "Prefer the project's Gradle Wrapper.",
		"JDK oder die in Android Studio enthaltene JBR verwenden.":        "Use a JDK or the JBR bundled with Android Studio.",
		"JDK oder Android Studio JBR installieren.":                       "Install a JDK or the Android Studio JBR.",
		"Git for Windows oder eine portable MinGit-Version installieren.": "Install Git for Windows or a portable MinGit distribution.",
		"GitHub CLI installieren; Login interaktiv mit gh auth login.":    "Install GitHub CLI; sign in interactively with gh auth login.",
		"Node.js LTS installieren.":                                       "Install Node.js LTS.",
		"Wird mit Node.js installiert.":                                   "Installed together with Node.js.",
		"Wird mit npm installiert.":                                       "Installed together with npm.",
		"Python installieren und dem PATH hinzufügen.":                    "Install Python and add it to PATH.",
		"pip mit Python installieren.":                                    "Install pip together with Python.",
		"Go installieren.":                                                "Install Go.",
		".NET SDK installieren.":                                          "Install the .NET SDK.",
		"Rust über rustup installieren.":                                  "Install Rust through rustup.",
		"Docker Desktop installieren und starten.":                        "Install and start Docker Desktop.",
		"CMake installieren.":                                             "Install CMake.",
		"Ninja installieren oder die Android-SDK-Kopie verwenden.":        "Install Ninja or use the copy bundled with the Android SDK.",
		"Windows OpenSSH Client Feature installieren.":                    "Install the Windows OpenSSH Client feature.",
		"curl installieren oder die Windows-Systemkopie verwenden.":       "Install curl or use the Windows system copy.",
		"Visual Studio Build Tools installieren.":                         "Install Visual Studio Build Tools.",
		"Visual Studio installieren.":                                     "Install Visual Studio.",
		"vswhere wird mit Visual Studio Installer installiert.":           "vswhere is installed with Visual Studio Installer.",
		"NuGet CLI installieren oder dotnet restore verwenden.":           "Install NuGet CLI or use dotnet restore.",
	}
	if translated, ok := translations[text]; ok {
		return translated
	}
	return text
}
