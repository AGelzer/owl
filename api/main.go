package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"

	"github.com/jackc/pgx"
)

var db *pgx.ConnPool

type (
	// Website ...
	Website struct {
		ID        int    `json:"id"`
		URL       string `json:"url"`
		LastCheck *Check `json:"lastCheck,omitempty"`
	}

	// Breakdown ...
	Breakdown struct {
		DNS         time.Duration `json:"dns"`
		Connection  time.Duration `json:"connection"`
		TLS         time.Duration `json:"tls"`
		Application time.Duration `json:"application"`
	}

	// Check ...
	Check struct {
		ID         int           `json:"id,omitempty"`
		Checked    time.Time     `json:"checkedAt"`
		WebsiteID  int           `json:"websiteId,omitempty"`
		StatusCode int           `json:"statusCode"`
		Duration   time.Duration `json:"duration"`
		Breakdown  *Breakdown    `json:"breakdown"`
	}

	// Entry ...
	Entry struct {
		Started         time.Time     `json:"startedAt"`
		Period          time.Duration `json:"period"`
		StatusCode      int           `json:"statusCode"`
		AverageDuration time.Duration `json:"averageDuration"`
		Checks          int64         `json:"totalChecks"`
	}
)

func handleNewWebsite(c echo.Context) error {
	website := &Website{}
	if err := c.Bind(website); err != nil {
		panic(err)
	}
	sql := `insert into websites (url) values ($1) returning id;`
	if err := db.QueryRow(sql, website.URL).Scan(&website.ID); err != nil {
		panic(err)
	}
	if err := c.JSON(http.StatusCreated, website); err != nil {
		panic(err)
	}
	return nil
}

func handleGetWebsite(c echo.Context) error {
	website := &Website{
		LastCheck: &Check{
			Breakdown: &Breakdown{},
		},
	}
	sql := `select id, url from websites where id = $1 limit 1;`
	if err := db.QueryRow(sql, c.Param("id")).Scan(&website.ID, &website.URL); err != nil {
		if err == pgx.ErrNoRows {
			return echo.NewHTTPError(http.StatusUnauthorized)
		}
		panic(err)
	}
	sql = `select checked_at, status_code, duration, breakdown from checks where website_id = $1 order by checked_at desc limit 1;`
	if err := db.QueryRow(sql, c.Param("id")).Scan(&website.LastCheck.Checked, &website.LastCheck.StatusCode, &website.LastCheck.Duration, &website.LastCheck.Breakdown); err != nil {
		if err == pgx.ErrNoRows {
			website.LastCheck = nil
		} else {
			panic(err)
		}
	}
	if err := c.JSON(http.StatusOK, website); err != nil {
		panic(err)
	}
	return nil
}

func handleGetWebsiteUptime(c echo.Context) error {
	mo := c.QueryParam("mo")
	if mo == "" {
		if err := c.JSON(http.StatusBadRequest, nil); err != nil {
			panic(err)
		}
		return nil
	}
	uptime := 0.0
	sql := `select percentage(sum(case when status_code = 200 then 1 else 0 end), count(status_code)) from checks where website_id = $1 and timestamptz_in_of(checked_at, '1 month -1 second', date_trunc('month', $2::timestamptz));`
	if err := db.QueryRow(sql, c.Param("id"), "1 "+mo).Scan(&uptime); err != nil {
		panic(err)
	}
	if err := c.JSON(http.StatusOK, uptime); err != nil {
		panic(err)
	}
	return nil
}

func handleListWebsites(c echo.Context) error {
	websites := []*Website{}
	sql := `select id, url from websites;`
	q, err := db.Query(sql)
	if err != nil {
		panic(err)
	}
	defer q.Close()
	for q.Next() {
		website := &Website{}
		err := q.Scan(&website.ID, &website.URL)
		if err != nil {
			panic(err)
		}
		websites = append(websites, website)
	}
	if err := c.JSON(http.StatusOK, websites); err != nil {
		panic(err)
	}
	return nil
}

func handleNewCheck(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		panic(nil)
	}
	check := &Check{
		WebsiteID: id,
	}
	if err := c.Bind(check); err != nil {
		panic(err)
	}
	sql := `insert into checks (website_id, status_code, duration, breakdown) values ($1, $2, $3, $4) returning id, checked_at;`
	if err := db.QueryRow(sql, check.WebsiteID, check.StatusCode, check.Duration, check.Breakdown).Scan(&check.ID, &check.Checked); err != nil {
		panic(err)
	}
	if err := c.JSON(http.StatusCreated, check); err != nil {
		panic(err)
	}
	return nil
}

func handleListChecks(c echo.Context) error {
	mo := c.QueryParam("mo")
	if mo == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing parameter 'mo'")
	}
	sql := `select id, checked_at, status_code, duration, breakdown from checks where website_id = $1 and timestamptz_in_of(checked_at, '1 month -1 second', date_trunc('month', $2::timestamptz)) order by checked_at desc;`
	q, err := db.Query(sql, c.Param("id"), "1 "+mo)
	if err != nil {
		panic(err)
	}
	defer q.Close()
	checks := []*Check{}
	for q.Next() {
		check := &Check{
			Breakdown: &Breakdown{},
		}
		q.Scan(&check.ID, &check.Checked, &check.StatusCode, &check.Duration, &check.Breakdown)
		checks = append(checks, check)
	}
	if err := c.JSON(http.StatusOK, checks); err != nil {
		panic(err)
	}
	return nil
}

func handleListHistory(c echo.Context) error {
	mo := c.QueryParam("mo")
	if mo == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing parameter 'mo'")
	}
	sql := `select checked_at, status_code, duration from checks where website_id = $1 and timestamptz_in_of(checked_at, '1 month -1 second', date_trunc('month', $2::timestamptz)) order by checked_at desc;`
	q, err := db.Query(sql, c.Param("id"), "1 "+mo)
	if err != nil {
		panic(err)
	}
	defer q.Close()
	lastEntry := &Entry{}
	entries := []*Entry{}
	for q.Next() {
		check := &Check{
			Breakdown: &Breakdown{},
		}
		q.Scan(&check.Checked, &check.StatusCode, &check.Duration)
		if check.StatusCode == lastEntry.StatusCode {
			lastEntry.Period = lastEntry.Started.Sub(check.Checked) / time.Minute
			lastEntry.Checks++
			lastEntry.AverageDuration += (check.Duration - lastEntry.AverageDuration) / time.Duration(lastEntry.Checks)
		} else {
			entry := &Entry{
				Started:         check.Checked,
				StatusCode:      check.StatusCode,
				Checks:          1,
				AverageDuration: check.Duration,
			}
			entries = append(entries, entry)
			lastEntry = entry
		}
	}
	if err := c.JSON(http.StatusOK, entries); err != nil {
		panic(err)
	}
	return nil
}

func main() {
	config, err := pgx.ParseURI(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	db, err = pgx.NewConnPool(pgx.ConnPoolConfig{
		ConnConfig: config,
	})
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${id} ${method} ${uri} ${status} ${latency_human} ${remote_ip} ${user_agent}\n",
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.CORS())

	e.GET("/websites", handleListWebsites)
	e.GET("/websites/:id", handleGetWebsite)
	e.GET("/websites/:id/uptime", handleGetWebsiteUptime)
	e.GET("/websites/:id/checks", handleListChecks)
	e.GET("/websites/:id/history", handleListHistory)

	authenticate := middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
		return key == os.Getenv("API_KEY"), nil
	})

	e.POST("/websites", handleNewWebsite, authenticate)
	e.POST("/websites/:id/checks", handleNewCheck, authenticate)

	shutdown := make(chan os.Signal, 2)
	signal.Notify(shutdown, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		// Wait for the signal.
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.Shutdown(ctx); err != nil {
			e.Logger.Fatal(err)
		}
		close(shutdown)
	}()

	e.Logger.Fatal(e.Start(":" + os.Getenv("PORT")))

	// Wait for shutdown or a second signal.
	<-shutdown
}
