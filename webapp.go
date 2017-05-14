package main

import (
    "encoding/json"
    "math/rand"
    "net/http"
    "sync"
    "time"
    "github.com/gin-gonic/gin"
)

type Card struct {
  Id 					int 	`json: id`
  Name 					string	`json: name`
  Rarity 				int 	`json: rarity`
  Monster_points		int		`json: monster_points`
  Jp_only				bool	`json: jp_only`
}

type Box struct {
  	sync.RWMutex
  	Cards *[]Card
  	Size int
}

type User struct {
  Name 	string
  Box Box
}

const userParam string = "user"

var cards []Card = getCards()
var users = struct{
	sync.RWMutex
	m map[string]User
}{m: make(map[string]User)}

func main() {
	rand.Seed(time.Now().Unix())

	r := gin.Default()
	r.GET("/roll", roll)
	r.GET("/scam", scam)
	r.GET("/status", status)
	r.GET("/keep", keep)
	r.Run() // listen and serve on 0.0.0.0:8080
}

func scam(ctx *gin.Context) {
	user := ctx.Query(userParam)
	if (user == "") {
		ctx.String(400, "Invalid user.")
		return
	}
	users.RLock()
	_, userExists := users.m[user]
	users.RUnlock()
	if (userExists) {
		ctx.String(400, user + " has already been scammed.")
		return
	}
	users.Lock()
	users.m[user] = User{user, Box{Cards: &[]Card{cards[0]}, Size: 1}}
	users.Unlock()
	ctx.String(200, user + " has been successfully scammed.")
}

func roll(ctx *gin.Context) {
  	user := ctx.Query(userParam)
  	if (user == "") {
  		ctx.String(400, "Invalid user.")
  		return
  	}
  	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if (!userExists) {
		ctx.String(400, user + " has not been scammed yet.")
  		return
	}
	// if (len(*userInfo.Box.Cards) > userInfo.Box.Size) {
	// 	ctx.String(400, user + "'s box space is full.")
	// 	return
	// }
  	var roll Card = cards[rand.Intn(len(cards))]
  	var resp = getEggTier(roll) + " " + roll.Name
  	userInfo.Box.Lock()
  	*userInfo.Box.Cards = append((*userInfo.Box.Cards)[0:1], roll)
  	userInfo.Box.Unlock()
  	ctx.String(200, resp)
}

func status(ctx *gin.Context) {
	user := ctx.Query(userParam)
  	if (user == "") {
  		ctx.String(400, "Invalid user.")
  		return
  	}
  	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if (!userExists) {
		ctx.String(400, user + " has not been scammed yet.")
  		return
	}
	userInfo.Box.RLock()
	var resp = user + "'s box: ["
	for i, card := range *userInfo.Box.Cards {
		if (i == 0) {
			resp = resp + card.Name + " (leader)"
		} else if (i == userInfo.Box.Size) {
			resp = resp + card.Name + " (overflow)"
		} else {
			resp = resp + card.Name
		}
		if (i < len(*userInfo.Box.Cards) - 1) {
			resp = resp + ", "
		}
	}
	resp = resp + "]"
	userInfo.Box.RUnlock()
	ctx.String(200, resp)
}

func keep(ctx *gin.Context) {
    user := ctx.Query(userParam)
  	if (user == "") {
  		ctx.String(400, "Invalid user.")
  		return
  	}
  	users.RLock()
	userInfo, userExists := users.m[user]
	users.RUnlock()
	if (!userExists) {
		ctx.String(400, user + " has not been scammed yet.")
  		return
	}
	userInfo.Box.Lock()
	if (len(*userInfo.Box.Cards) < 2) {
		userInfo.Box.Unlock()
		ctx.String(400, user + " does not have a new card to keep.")
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
	if (err != nil) {
		panic(err.Error())
	}
	
	defer resp.Body.Close()
	
	decoder := json.NewDecoder(resp.Body)
    var allCards []Card
    err = decoder.Decode(&allCards)
	if (err != nil) {
		panic(err.Error())
	}

	return filterCards(allCards)
}

func filterCards(cards []Card) (ret []Card) {
	for _, card := range cards {
		if (!card.Jp_only) {
			ret = append(ret, card)
		}
	}
	return ret
}

func getEggTier(card Card) string {
	if (card.Monster_points >= 50000 || card.Rarity > 8) {
		return "DIAMOND EGG!!!"
	}
	if (card.Rarity > 6) {
		return "GOLD EGG!!"
	}
	if (card.Rarity > 4) {
		return "SILVER EGG!"
	}
	return "BRONZE EGG"
}