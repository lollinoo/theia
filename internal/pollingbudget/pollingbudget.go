package pollingbudget

import (
	"strconv"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
)

const defaultWorkerPoolSize = 5

type Getter interface {
	Get(string) (string, error)
}

func Resolve(repo Getter) map[domain.VolatilityClass]int {
	total := legacyTotal(repo)

	budgets := defaultSplit(total)
	if repo == nil {
		return budgets
	}

	if value, ok := readPositive(repo, domain.SettingSNMPWorkerPoolPerformance); ok {
		budgets[domain.VolatilityClassPerformance] = value
	}
	if value, ok := readPositive(repo, domain.SettingSNMPWorkerPoolOperational); ok {
		budgets[domain.VolatilityClassOperational] = value
	}
	if value, ok := readPositive(repo, domain.SettingSNMPWorkerPoolStatic); ok {
		budgets[domain.VolatilityClassStatic] = value
	}

	return budgets
}

func Total(repo Getter) int {
	return Sum(Resolve(repo))
}

func Sum(budgets map[domain.VolatilityClass]int) int {
	total := 0
	for _, volatility := range orderedVolatilityClasses() {
		if budgets[volatility] > 0 {
			total += budgets[volatility]
		}
	}
	if total <= 0 {
		return defaultWorkerPoolSize
	}
	return total
}

func Clamp(budgets map[domain.VolatilityClass]int, limit int) map[domain.VolatilityClass]int {
	clamped := make(map[domain.VolatilityClass]int, len(budgets))
	for _, volatility := range orderedVolatilityClasses() {
		clamped[volatility] = budgets[volatility]
	}
	if limit <= 0 {
		for _, volatility := range orderedVolatilityClasses() {
			clamped[volatility] = 0
		}
		return clamped
	}

	if Sum(clamped) <= limit {
		return clamped
	}

	active := make([]domain.VolatilityClass, 0, len(clamped))
	for _, volatility := range orderedVolatilityClasses() {
		if clamped[volatility] > 0 {
			active = append(active, volatility)
			clamped[volatility] = 0
		}
	}

	remaining := limit
	for _, volatility := range active {
		if remaining == 0 {
			break
		}
		clamped[volatility] = 1
		remaining--
	}

	for _, volatility := range []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	} {
		for remaining > 0 && clamped[volatility] < budgets[volatility] {
			clamped[volatility]++
			remaining--
		}
	}

	return clamped
}

func legacyTotal(repo Getter) int {
	if repo == nil {
		return defaultWorkerPoolSize
	}
	value, err := repo.Get(domain.SettingSNMPWorkerPoolSize)
	if err != nil {
		return defaultWorkerPoolSize
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return defaultWorkerPoolSize
	}
	return domain.CoerceConstrainedInt(domain.SettingSNMPWorkerPoolSize, value, defaultWorkerPoolSize)
}

func readPositive(repo Getter, key string) (int, bool) {
	value, err := repo.Get(key)
	if err != nil {
		return 0, false
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return domain.CoerceConstrainedInt(key, value, 0), true
}

func defaultSplit(total int) map[domain.VolatilityClass]int {
	budgets := map[domain.VolatilityClass]int{
		domain.VolatilityClassPerformance: 0,
		domain.VolatilityClassOperational: 0,
		domain.VolatilityClassStatic:      0,
	}

	switch {
	case total <= 0:
		budgets[domain.VolatilityClassPerformance] = defaultWorkerPoolSize
	case total == 1:
		budgets[domain.VolatilityClassPerformance] = 1
	case total == 2:
		budgets[domain.VolatilityClassPerformance] = 1
		budgets[domain.VolatilityClassOperational] = 1
	default:
		budgets[domain.VolatilityClassPerformance] = total - 2
		budgets[domain.VolatilityClassOperational] = 1
		budgets[domain.VolatilityClassStatic] = 1
	}

	return budgets
}

func orderedVolatilityClasses() []domain.VolatilityClass {
	return []domain.VolatilityClass{
		domain.VolatilityClassPerformance,
		domain.VolatilityClassOperational,
		domain.VolatilityClassStatic,
	}
}
