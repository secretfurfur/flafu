package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"math/rand"
	"net/http"
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

type Box struct {
	sync.RWMutex
	Cards *[]Card
	Size  int
}

type User struct {
	Name string
	Box  Box
}

type ShouterUi struct {
	Name string
	Leader Card
	Message string
}

type SupporterUi struct {
	Name   string
	Leader Card
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
	return SupporterUi{s.User.Name, (*s.User.Box.Cards)[0]}
}

const userParam string = "user"
const messageParam string = "message"

var cards []Card = getCards()
var users = struct {
	sync.RWMutex
	m map[string]User
}{m: make(map[string]User)}

var supporters = struct {
	sync.RWMutex
	m map[string]*Supporter
}{m: make(map[string]*Supporter)}

var shouters = make(chan ShouterUi, 100)

func main() {
	rand.Seed(time.Now().Unix())
	fmt.Println("Starting server")

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
	ctx.HTML(200, "shouts.tmpl", gin.H{"Shout":s})
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
	var shout = ShouterUi{userInfo.Name, (*userInfo.Box.Cards)[0], message}
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
	ctx.String(200, user+" is now supporting Sweetily with "+(*userInfo.Box.Cards)[0].Name+"!")
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
	users.Lock()
	users.m[user] = User{user, Box{Cards: &[]Card{cards[0]}, Size: 1}}
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
	var roll Card = cards[rand.Intn(len(cards))]
	var resp = user + "'s roll: " + getEggTier(roll) + " " + roll.Name
	userInfo.Box.Lock()
	*userInfo.Box.Cards = append((*userInfo.Box.Cards)[0:1], roll)
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
	for i, card := range *userInfo.Box.Cards {
		if i == 0 {
			resp = resp + card.Name + " (leader)"
		} else if i == userInfo.Box.Size {
			resp = resp + card.Name + " (overflow)"
		} else {
			resp = resp + card.Name
		}
		if i < len(*userInfo.Box.Cards)-1 {
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
	if len(*userInfo.Box.Cards) < 2 {
		userInfo.Box.Unlock()
		ctx.String(200, user+" does not have a new card to keep.")
		return
	}
	(*userInfo.Box.Cards)[0] = (*userInfo.Box.Cards)[1]
	*userInfo.Box.Cards = (*userInfo.Box.Cards)[:1]
	var resp = user + "'s new leader is: " + (*userInfo.Box.Cards)[0].Name
	userInfo.Box.Unlock()
	ctx.String(200, resp)
}

func getCards() (ret []Card) {
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

	return filterCards(allCards)
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
