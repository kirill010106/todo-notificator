package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type RegisterReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResp struct {
	AccessToken string `json:"access_token"`
}

type TaskReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func main() {
	baseURL := flag.String("url", "http://localhost:8082", "API base URL")
	concurrency := flag.Int("c", 10, "Number of concurrent users")
	duration := flag.Duration("d", 20*time.Second, "Test duration")
	flag.Parse()

	fmt.Printf("🚀 Запуск стресс-теста бизнес-логики (Users + Tasks + DB)\n")
	fmt.Printf("Параллельных пользователей: %d, Время: %v\n\n", *concurrency, *duration)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        500,
			MaxIdleConnsPerHost: 100,
		},
	}

	var (
		regCount   uint64
		taskCount  uint64
		errorCount uint64
		stopCh     = make(chan struct{})
		wg         sync.WaitGroup
	)

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-stopCh:
					return
				default:
					// 1. Регистрация нового уникального пользователя
					email := fmt.Sprintf("load_%d_%d@test.com", workerID, rand.Int63())
					pass := "Secret123!"

					regBody, _ := json.Marshal(RegisterReq{Email: email, Password: pass})
					resp, err := client.Post(*baseURL+"/api/v1/register", "application/json", bytes.NewReader(regBody))
					if err != nil || resp.StatusCode != http.StatusOK {
						atomic.AddUint64(&errorCount, 1)
						if resp != nil {
							resp.Body.Close()
						}
						time.Sleep(100 * time.Millisecond)
						continue
					}
					resp.Body.Close()
					atomic.AddUint64(&regCount, 1)

					// 2. Логин и получение токена
					resp, err = client.Post(*baseURL+"/api/v1/login", "application/json", bytes.NewReader(regBody))
					if err != nil || resp.StatusCode != http.StatusOK {
						atomic.AddUint64(&errorCount, 1)
						if resp != nil {
							resp.Body.Close()
						}
						continue
					}
					var authResp LoginResp
					_ = json.NewDecoder(resp.Body).Decode(&authResp)
					resp.Body.Close()

					if authResp.AccessToken == "" {
						atomic.AddUint64(&errorCount, 1)
						continue
					}

					// 3. Создание 3 задач в цикле
					for t := 0; t < 3; t++ {
						taskBody, _ := json.Marshal(TaskReq{
							Title:       fmt.Sprintf("Задача #%d", rand.Intn(10000)),
							Description: "Нагрузочное тестирование через Grafana & Prometheus",
						})
						req, _ := http.NewRequest("POST", *baseURL+"/api/v1/tasks", bytes.NewReader(taskBody))
						req.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
						req.Header.Set("Content-Type", "application/json")

						tResp, err := client.Do(req)
						if err != nil || tResp.StatusCode != http.StatusOK {
							atomic.AddUint64(&errorCount, 1)
						} else {
							atomic.AddUint64(&taskCount, 1)
						}
						if tResp != nil {
							tResp.Body.Close()
						}
					}

					// 4. Получение списка всех задач пользователя
					getReq, _ := http.NewRequest("GET", *baseURL+"/api/v1/tasks", nil)
					getReq.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
					gResp, err := client.Do(getReq)
					if err != nil || gResp.StatusCode != http.StatusOK {
						atomic.AddUint64(&errorCount, 1)
					}
					if gResp != nil {
						gResp.Body.Close()
					}
				}
			}
		}(i)
	}

	time.Sleep(*duration)
	close(stopCh)
	wg.Wait()

	fmt.Printf("\n🏁 Результаты теста:\n")
	fmt.Printf("✅ Зарегистрировано пользователей: %d\n", atomic.LoadUint64(&regCount))
	fmt.Printf("✅ Создано задач в БД:           %d\n", atomic.LoadUint64(&taskCount))
	fmt.Printf("❌ Ошибок (или 429 RateLimit):     %d\n", atomic.LoadUint64(&errorCount))
}
