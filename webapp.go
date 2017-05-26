package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type Card struct {
	Id             int    `json: id`
	Name           string `json: name`
	Rarity         int    `json: rarity`
	Monster_points int    `json: monster_points`
	Jp_only        bool   `json: jp_only`
}

type UserCard struct {
	Key int
	Id  int
}

type Box struct {
	sync.RWMutex
	UserCards *[]UserCard
	Size      int
}

type User struct {
	Name string
	Box  Box
}

type ShouterUi struct {
	Name    string
	Leader  UserCard
	Message string
}

type SupporterUi struct {
	Name   string
	Leader UserCard
}

type Supporter struct {
	User User
	Ttl  int
}

func (s Supporter) expired() bool {
	return s.Ttl <= 0
}

func (s *Supporter) tick() {
	s.Ttl = s.Ttl - 1
}

func (s Supporter) toUi() SupporterUi {
	return SupporterUi{s.User.Name, (*s.User.Box.UserCards)[0]}
}

// SQL
const createUsers string = `
	CREATE TABLE IF NOT EXISTS Users(
		name TEXT PRIMARY KEY NOT NULL,
		cards INT NOT NULL
	)`
const createUserCards string = `
	CREATE TABLE IF NOT EXISTS UserCards(
		key SERIAL PRIMARY KEY NOT NULL,
		id INT NOT NULL
	)`
const selectUsers string = `SELECT * FROM Users`
const insertUserCard string = `INSERT INTO UserCards (id) VALUES ($1) RETURNING key`
const deleteUserCard string = `DELETE FROM UserCards Where key = $1`
const insertUser string = `INSERT INTO Users (name, cards) VALUES ($1, $2)`
const updateUser string = `UPDATE Users SET (cards) = ($1) WHERE name = $2`

const userParam string = "user"
const messageParam string = "message"

var validIds []int = []int{}
var cards map[int]Card = getCards()
var users = struct {
	sync.RWMutex
	m map[string]User
}{m: make(map[string]User)}

var supporters = struct {
	sync.RWMutex
	m map[string]*Supporter
}{m: make(map[string]*Supporter)}

var shouters = make(chan ShouterUi, 100)

var db *sql.DB

func bootstrapDB() {
	var err error
	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL")+"?sslmode=disable")
	if err != nil {
		panic(err)
	}

	_, err = db.Query(createUsers)
	if err != nil {
		panic(err)
	}
	_, err = db.Query(createUserCards)
	if err != nil {
		panic(err)
	}

	rows, err2 := db.Query(selectUsers)
	if err2 != nil {
		panic(err2)
	}
	type row struct {
		name  string
		cards []int
	}
	for rows.Next() {
		var next row
		var leaderid int
		err = rows.Scan(&next.name, &leaderid)
		next.cards = []int{leaderid}
		if err != nil {
			panic(err)
		}
		var userCards []UserCard = []UserCard{}
		for _, id := range next.cards {
			res, err3 := db.Query("SELECT * from UserCards where key =" + strconv.Itoa(id))
			if err3 != nil {
				panic(err3)
			}
			for res.Next() {
				var userCard UserCard
				err = res.Scan(&userCard.Key, &userCard.Id)
				if err != nil {
					panic(err)
				}
				userCards = append(userCards, userCard)
			}
		}
		var user User = User{Name: next.name, Box: Box{UserCards: &userCards, Size: len(userCards)}}
		users.m[user.Name] = user
	}
}

func main() {
	rand.Seed(time.Now().Unix())
	fmt.Println("Starting server")

	bootstrapDB()

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/css", "./css")
	r.Static("/assets", "./assets")

	// External commands
	r.GET("/roll", roll)
	r.GET("/scam", scam)
	r.GET("/status", status)
	r.GET("/keep", keep)
	r.GET("/support", support)
	r.GET("/shout", shout)

	// Internal commands
	r.GET("/supports", supports)
	r.GET("/shouts", shouts)

	// Views
	r.GET("/viewsupports", viewSupports)
	r.GET("/viewshouts", viewShouts)

	r.Run() // listen and serve on 0.0.0.0:8080
}

func viewSupports(ctx *gin.Context) {
	ctx.HTML(200, "viewsupports.tmpl", nil)
}

func viewShouts(ctx *gin.Context) {
	ctx.HTML(200, "viewshouts.tmpl", nil)
}

func shouts(ctx *gin.Context) {
	if len(shouters) == 0 {
		ctx.Status(200)
		return
	}
	s := <-shouters
	ctx.HTML(200, "shouts.tmpl", gin.H{"Shout": s})
}

func shout(ctx *gin.Context) {
	user := ctx.Query(userParam)
	message := ctx.Query(messageParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if !userExists {
		ctx.String(200, user+" has not been scammed yet.")
		return
	}
	if len(message) > 100 {
		ctx.String(200, user+" your message cannot be longer than 100 characters.")
		return
	}
	var shout = ShouterUi{userInfo.Name, (*userInfo.Box.UserCards)[0], message}
	shouters <- shout
	ctx.String(200, user+"'s message has been queued.")
}

func supports(ctx *gin.Context) {
	supporters.RLock()
	var u = make(map[string]SupporterUi)
	for user, support := range supporters.m {
		support.tick()
		if support.expired() {
			delete(supporters.m, user)
		} else {
			u[user] = support.toUi()
		}
	}
	ctx.HTML(200, "supports.tmpl", gin.H{"Supports": u})
	supporters.RUnlock()
}

func support(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if !userExists {
		ctx.String(200, user+" has not been scammed yet.")
		return
	}
	supporters.RLock()
	_, alreadySupporting := supporters.m[user]
	num := len(supporters.m)
	supporters.RUnlock()
	if alreadySupporting {
		ctx.String(200, user+" is already supporting.")
		return
	}
	if num >= 1 {
		ctx.String(200, "Sweetily has too many supporters right now!")
		return
	}
	supporters.Lock()
	supporters.m[user] = &Supporter{userInfo, 12}
	supporters.Unlock()
	ctx.String(200, user+" is now supporting Sweetily with "+cards[(*userInfo.Box.UserCards)[0].Id].Name+"!")
}

func scam(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	_, userExists := users.m[user]
	users.RUnlock()
	if userExists {
		ctx.String(200, user+" has already been scammed.")
		return
	}

	var starterId = 1
	var key int
	err := db.QueryRow(insertUserCard, starterId).Scan(&key)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(insertUser, user, key)
	if err != nil {
		panic(err)
	}

	users.Lock()
	users.m[user] = User{user, Box{UserCards: &[]UserCard{UserCard{Key: key, Id: starterId}}, Size: 1}}
	users.Unlock()
	ctx.String(200, user+" has been successfully scammed.")
}

func roll(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if !userExists {
		ctx.String(200, user+" has not been scammed yet.")
		return
	}
	// if (len(*userInfo.Box.Cards) > userInfo.Box.Size) {
	// 	ctx.String(200, user + "'s box space is full.")
	// 	return
	// }
	var roll Card = cards[validIds[rand.Intn(len(validIds))]]
	var resp = user + "'s roll: " + getEggTier(roll) + " " + roll.Name
	var newCard UserCard = UserCard{Key: -1, Id: roll.Id}
	userInfo.Box.Lock()
	*userInfo.Box.UserCards = append((*userInfo.Box.UserCards)[0:1], newCard)
	userInfo.Box.Unlock()
	ctx.String(200, resp)
}

func status(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if !userExists {
		ctx.String(200, user+" has not been scammed yet.")
		return
	}
	userInfo.Box.RLock()
	var resp = user + "'s box: ["
	for i, card := range *userInfo.Box.UserCards {
		if i == 0 {
			resp = resp + cards[card.Id].Name + " (leader)"
		} else if i == userInfo.Box.Size {
			resp = resp + cards[card.Id].Name + " (overflow)"
		} else {
			resp = resp + cards[card.Id].Name
		}
		if i < len(*userInfo.Box.UserCards)-1 {
			resp = resp + ", "
		}
	}
	resp = resp + "]"
	userInfo.Box.RUnlock()
	ctx.String(200, resp)
}

func keep(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if user == "" {
		ctx.String(200, "Invalid user.")
		return
	}
	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if !userExists {
		ctx.String(200, user+" has not been scammed yet.")
		return
	}
	userInfo.Box.Lock()
	if len(*userInfo.Box.UserCards) < 2 {
		userInfo.Box.Unlock()
		ctx.String(200, user+" does not have a new card to keep.")
		return
	}

	var key int
	err := db.QueryRow(insertUserCard, (*userInfo.Box.UserCards)[1].Id).Scan(&key)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(updateUser, key, user)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(deleteUserCard, (*userInfo.Box.UserCards)[0].Key)
	if err != nil {
		panic(err)
	}

	(*userInfo.Box.UserCards)[1].Key = key
	(*userInfo.Box.UserCards)[0] = (*userInfo.Box.UserCards)[1]
	*userInfo.Box.UserCards = (*userInfo.Box.UserCards)[:1]
	var resp = user + "'s new leader is: " + cards[(*userInfo.Box.UserCards)[0].Id].Name
	userInfo.Box.Unlock()
	ctx.String(200, resp)
}

func getCards() map[int]Card {
	resp, err := http.Get("https://www.padherder.com/api/monsters/")
	if err != nil {
		panic(err.Error())
	}

	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var allCards []Card
	err = decoder.Decode(&allCards)
	if err != nil {
		panic(err.Error())
	}

	ret := make(map[int]Card)
	for _, card := range filterCards(allCards) {
		ret[card.Id] = card
		validIds = append(validIds, card.Id)
	}
	return ret
}

func filterCards(cards []Card) (ret []Card) {
	for _, card := range cards {
		if !card.Jp_only {
			ret = append(ret, card)
		}
	}
	return ret
}

func getEggTier(card Card) string {
	if card.Monster_points >= 15000 || card.Rarity > 8 {
		return "DIAMOND EGG!!!"
	}
	if card.Monster_points >= 5000 || card.Rarity > 6 {
		return "GOLD EGG!!"
	}
	if card.Monster_points >= 3000 || card.Rarity > 4 {
		return "SILVER EGG!"
	}
	return "BRONZE EGG"
}
