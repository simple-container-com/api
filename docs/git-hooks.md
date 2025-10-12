# Git Hooks Documentation

## Pre-commit Hook

Автоматически запускается перед каждым коммитом для обеспечения качества кода.

### Что проверяется

1. **Go Tests** - Все тесты должны пройти
   - Запускается: `go test ./... -v -race -coverprofile=coverage.out`
   - Показывает покрытие кода тестами

2. **Code Formatting** - Код должен быть отформатирован
   - Проверяется: `gofmt -l .`
   - Исправить: `gofmt -w .`

3. **Go Vet** - Статический анализ кода
   - Запускается: `go vet ./...`
   - Находит потенциальные ошибки

4. **Golangci-lint** (опционально)
   - Запускается если установлен
   - Установка: `brew install golangci-lint` (macOS)

### Использование

#### Обычный коммит
```bash
git add .
git commit -m "feat: add new feature"
# Hook автоматически запустится
```

#### Обход hook (только для экстренных случаев!)
```bash
git commit -m "WIP: emergency fix" --no-verify
```

⚠️ **Внимание:** Используйте `--no-verify` только в крайних случаях!

### Что делать если тесты не проходят

1. **Посмотрите вывод ошибки:**
   ```bash
   # Hook покажет какие тесты упали
   ```

2. **Запустите тесты вручную:**
   ```bash
   go test ./... -v
   ```

3. **Исправьте ошибки**

4. **Попробуйте снова:**
   ```bash
   git commit -m "fix: исправлены тесты"
   ```

### Что делать если форматирование неправильное

```bash
# Автоматически исправить форматирование
gofmt -w .

# Добавить изменения
git add .

# Коммитить
git commit -m "style: fix formatting"
```

### Установка hook в новом клоне

Hook уже находится в `.git/hooks/pre-commit` и автоматически активируется при клонировании.

Если нужно переустановить:
```bash
chmod +x .git/hooks/pre-commit
```

### Отключение hook

Если нужно временно отключить (не рекомендуется):

**Вариант 1: Для одного коммита**
```bash
git commit --no-verify -m "message"
```

**Вариант 2: Полное отключение**
```bash
mv .git/hooks/pre-commit .git/hooks/pre-commit.disabled
```

**Включить обратно:**
```bash
mv .git/hooks/pre-commit.disabled .git/hooks/pre-commit
```

### Статистика

После успешного прохождения hook показывает:
- ✅ Количество пройденных тестов
- 📊 Покрытие кода (Test Coverage)
- 🎨 Статус форматирования
- 🔍 Результаты go vet

### Пример успешного вывода

```
🧪 Running pre-commit tests...

📦 Running Go tests...
✅ All tests passed!

📊 Test Coverage: 12.8%

🎨 Checking code formatting...
✅ Code formatting is correct

🔍 Running go vet...
✅ go vet passed

⚠️  golangci-lint not installed (optional)

✅ All pre-commit checks passed! 🎉
```

### Пример неудачного вывода

```
🧪 Running pre-commit tests...

📦 Running Go tests...
❌ Tests failed! Commit aborted.

⚠️  Fix the failing tests before committing.
```

### Рекомендации

1. ✅ **Всегда запускайте тесты локально** перед коммитом
2. ✅ **Форматируйте код** с помощью `gofmt -w .`
3. ✅ **Пишите тесты** для нового кода
4. ❌ **Не используйте `--no-verify`** без крайней необходимости
5. ✅ **Следите за покрытием** - стремитесь к > 80%

### CI/CD Integration

Pre-commit hook дополняет CI/CD pipeline:
- **Local (pre-commit):** Быстрая проверка перед коммитом
- **CI/CD:** Полная проверка при push в GitHub

Оба уровня защиты обеспечивают качество кода! 🛡️
