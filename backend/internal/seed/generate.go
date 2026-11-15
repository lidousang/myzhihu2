package seed

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"myzhihu2/repository/model"
)

//go:embed showcase/*.json
var showcaseFS embed.FS

type showcaseAnswer struct {
	Author string `json:"author"`
	Body   string `json:"body"`
}

type showcaseItem struct {
	Author  string           `json:"author"`
	Kind    string           `json:"kind"`
	Title   string           `json:"title"`
	Body    string           `json:"body"`
	Hot     bool             `json:"hot"`
	Answers []showcaseAnswer `json:"answers"`
}

type showcaseFile struct {
	Topic string         `json:"topic"`
	Items []showcaseItem `json:"items"`
}

// Dataset is a complete, deterministic demo dataset ready to be inserted.
type Dataset struct {
	Users     []model.User
	Follows   []model.UserFollow
	Contents  []model.Content
	Reactions []model.Reaction
	Behaviors []model.BehaviorEvent

	// OnlineUsers get a pre-built inbox so pushes reach visible "active" fans.
	OnlineUsers []uint
	// HotContentID is the pinned big V article with many likes.
	HotContentID uint
	// ShowcaseCount is the number of reviewed showcase rows (roots + answers).
	ShowcaseCount int
}

// readerFollows is the demo reader's fixed follow list: two big Vs (pull) and
// six mid-tier authors across topics (push). Travel big V and every photo
// author are deliberately not followed.
var readerFollows = []uint{BigVTechID, BigVFoodID, midID("tech", 1), midID("tech", 2),
	midID("food", 1), midID("fitness", 1), midID("fitness", 2), midID("travel", 1)}

// interestAuthors are unfollowed photo authors whose posts the reader liked;
// they drive the interest stream ("topic:photo").
var interestAuthors = []uint{midID("photo", 1), midID("photo", 2), midID("photo", 3)}

func midID(topic string, n int) uint {
	return MidStartID + uint(slices.Index(topicOrder, topic)*6+n-1)
}

type generator struct {
	p   Profile
	r   *rand.Rand
	now time.Time

	ds        *Dataset
	userTopic map[uint]string   // authors' main topic
	prefs     map[uint][]string // ordinary users' preferred topics
	ordinary  []uint
	posters   []uint

	follows       map[[2]uint]bool
	followerCount map[uint]int
	followingOf   map[uint][]uint

	contentTopic  map[uint]string
	contentTime   map[uint]time.Time
	contentAuthor map[uint]uint
	eligible      []uint // content that can receive likes and collections
	reacted       map[[2]uint]map[string]bool
}

// Generate builds the dataset. The same profile, seed and now always produce
// the same dataset: every random choice comes from one PCG source and maps
// are never iterated without sorting.
func Generate(p Profile, seed uint64, now time.Time) (*Dataset, error) {
	g := &generator{
		p: p, r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), now: now.Truncate(time.Second),
		ds: &Dataset{}, userTopic: map[uint]string{}, prefs: map[uint][]string{},
		follows: map[[2]uint]bool{}, followerCount: map[uint]int{}, followingOf: map[uint][]uint{},
		contentTopic: map[uint]string{}, contentTime: map[uint]time.Time{}, contentAuthor: map[uint]uint{},
		reacted: map[[2]uint]map[string]bool{},
	}
	g.users()
	g.followGraph()
	g.fillerContent()
	if err := g.showcaseContent(); err != nil {
		return nil, err
	}
	g.reactions()
	g.readerBehavior()
	g.onlineUsers()
	for i := range g.ds.Users {
		g.ds.Users[i].FollowerCount = g.followerCount[g.ds.Users[i].ID]
	}
	return g.ds, nil
}

func (g *generator) between(r Range) int {
	if r.Max <= r.Min {
		return r.Min
	}
	return r.Min + g.r.IntN(r.Max-r.Min+1)
}

// ---------------------------------------------------------------- users --

func (g *generator) users() {
	base := g.now.AddDate(0, 0, -120)
	add := func(id uint, username, display, bio string) {
		t := base.Add(time.Duration(id%1000) * time.Minute)
		g.ds.Users = append(g.ds.Users, model.User{
			ID: id, Username: username, PasswordHash: UnusablePasswordHash,
			DisplayName: display, Bio: bio, CreatedAt: t, UpdatedAt: t,
		})
	}
	for _, id := range []uint{BigVTechID, BigVFoodID, BigVTravelID, ReaderID} {
		p := fixedPersonas[id]
		add(id, p.username, p.display, p.bio)
	}
	g.userTopic[BigVTechID], g.userTopic[BigVFoodID], g.userTopic[BigVTravelID] = model.TopicTech, model.TopicFood, model.TopicTravel
	for ti, topic := range topicOrder {
		for n := 0; n < 6; n++ {
			id := MidStartID + uint(ti*6+n)
			add(id, fmt.Sprintf("seed_m%d", id), midDisplayNames[topic][n],
				fmt.Sprintf("%s作者，分享%s相关的经验和思考。", topicLabel[topic], topicLabel[topic]))
			g.userTopic[id] = topic
		}
	}
	for i := 0; i < g.p.Ordinary; i++ {
		id := OrdinaryID + uint(i)
		first := topicOrder[g.r.IntN(len(topicOrder))]
		prefs := []string{first}
		if g.r.IntN(2) == 0 {
			if second := topicOrder[g.r.IntN(len(topicOrder))]; second != first {
				prefs = append(prefs, second)
			}
		}
		g.prefs[id] = prefs
		g.ordinary = append(g.ordinary, id)
		display := namePrefixes[g.r.IntN(len(namePrefixes))] + nameSuffixes[g.r.IntN(len(nameSuffixes))]
		labels := make([]string, len(prefs))
		for j, t := range prefs {
			labels[j] = topicLabel[t]
		}
		add(id, fmt.Sprintf("seed_u%d", id), display, "喜欢"+strings.Join(labels, "和")+"。")
		if g.r.Float64() < g.p.PosterRatio {
			g.posters = append(g.posters, id)
			g.userTopic[id] = first
		}
	}
}

// --------------------------------------------------------------- follows --

func (g *generator) follow(follower, following uint) bool {
	key := [2]uint{follower, following}
	if follower == following || g.follows[key] {
		return false
	}
	g.follows[key] = true
	g.followerCount[following]++
	g.followingOf[follower] = append(g.followingOf[follower], following)
	start := g.now.AddDate(0, 0, -100)
	t := start.Add(time.Duration(g.r.Int64N(int64(95 * 24 * time.Hour))))
	g.ds.Follows = append(g.ds.Follows, model.UserFollow{FollowerID: follower, FollowingID: following, CreatedAt: t})
	return true
}

// fillFollowers adds ordinary followers to author until target is reached,
// preferring users who like the author's topic (a simple preferential model).
func (g *generator) fillFollowers(author uint, topic string, target int) {
	var preferred, others []uint
	for _, id := range g.ordinary {
		if slices.Contains(g.prefs[id], topic) {
			preferred = append(preferred, id)
		} else {
			others = append(others, id)
		}
	}
	g.r.Shuffle(len(preferred), func(i, j int) { preferred[i], preferred[j] = preferred[j], preferred[i] })
	g.r.Shuffle(len(others), func(i, j int) { others[i], others[j] = others[j], others[i] })
	for _, id := range append(preferred, others...) {
		if g.followerCount[author] >= target {
			return
		}
		g.follow(id, author)
	}
}

func (g *generator) followGraph() {
	for _, id := range readerFollows {
		g.follow(ReaderID, id)
	}
	for n := 1; n <= 6; n++ {
		g.follow(midID("tech", n), BigVTechID)
		g.follow(midID("food", n), BigVFoodID)
		g.follow(midID("travel", n), BigVTravelID)
	}
	for _, id := range []uint{midID("tech", 3), midID("tech", 4), midID("tech", 5), BigVFoodID} {
		g.follow(BigVTechID, id)
	}
	g.follow(BigVFoodID, BigVTravelID)
	g.follow(BigVTravelID, BigVFoodID)

	for i, id := range []uint{BigVTechID, BigVFoodID, BigVTravelID} {
		g.fillFollowers(id, g.userTopic[id], g.between(g.p.BigVFollowers[i]))
	}
	for ti, topic := range topicOrder {
		for n := 0; n < 6; n++ {
			g.fillFollowers(MidStartID+uint(ti*6+n), topic, g.between(g.p.MidFollowers))
		}
	}
	if len(g.posters) > 0 {
		for _, id := range g.ordinary {
			for k := g.between(g.p.OrdinaryFollow); k > 0; k-- {
				g.follow(id, g.posters[g.r.IntN(len(g.posters))])
			}
		}
	}
}

// -------------------------------------------------------------- content --

func (g *generator) addContent(c model.Content) {
	c.UpdatedAt = c.CreatedAt
	g.ds.Contents = append(g.ds.Contents, c)
	g.contentTopic[c.ID] = c.Topic
	g.contentTime[c.ID] = c.CreatedAt
	g.contentAuthor[c.ID] = c.AuthorID
}

// fillerContent creates template posts aged 60 to 8 days, older (lower IDs)
// than every showcase post, so they never reach the first screen.
func (g *generator) fillerContent() {
	from, to := g.now.AddDate(0, 0, -60), g.now.AddDate(0, 0, -8)
	span := to.Sub(from)
	mids := make([]uint, 0, 30)
	for i := 0; i < 30; i++ {
		mids = append(mids, MidStartID+uint(i))
	}
	for i := 0; i < g.p.Filler; i++ {
		author := mids[g.r.IntN(len(mids))]
		if len(g.posters) > 0 && g.r.Float64() >= 0.4 {
			author = g.posters[g.r.IntN(len(g.posters))]
		}
		topic := g.userTopic[author]
		subject := fillerSubjects[topic][g.r.IntN(len(fillerSubjects[topic]))]
		kind, title := model.ContentKindArticle, subject+fillerSuffixes[g.r.IntN(len(fillerSuffixes))]
		if g.r.IntN(4) == 0 {
			kind, title = model.ContentKindQuestion, subject+"有什么经验可以分享？"
		}
		g.addContent(model.Content{
			ID: FillerStartID + uint(i), AuthorID: author, Kind: kind, Topic: topic, Title: title,
			Body:      g.fillerBody(topic),
			CreatedAt: from.Add(span * time.Duration(i+1) / time.Duration(g.p.Filler+1)),
		})
	}
}

// fillerBody draws a log-normal number of sentences (about 200-2000 CJK
// characters) so row sizes resemble real posts for load testing.
func (g *generator) fillerBody(topic string) string {
	n := int(math.Exp(g.r.NormFloat64()*0.6 + 3.3))
	n = min(max(n, 11), 110)
	bank := fillerSentences[topic]
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(bank[g.r.IntN(len(bank))])
	}
	return b.String()
}

func (g *generator) resolveAuthor(key, topic string) (uint, error) {
	switch key {
	case "bigv_tech":
		return BigVTechID, nil
	case "bigv_food":
		return BigVFoodID, nil
	case "bigv_travel":
		return BigVTravelID, nil
	case "ordinary":
		for tries := 0; tries < 50; tries++ {
			id := g.ordinary[g.r.IntN(len(g.ordinary))]
			if slices.Contains(g.prefs[id], topic) {
				return id, nil
			}
		}
		return g.ordinary[g.r.IntN(len(g.ordinary))], nil
	}
	var t string
	var n int
	if _, err := fmt.Sscanf(strings.ReplaceAll(key, "_", " "), "mid %s %d", &t, &n); err != nil || n < 1 || n > 6 || !model.ValidTopic(t) {
		return 0, fmt.Errorf("showcase: unknown author key %q", key)
	}
	return midID(t, n), nil
}

// showcaseClass decides the reader's first-screen rotation:
// A big V tech (pull) / B unfollowed photo authors (interest) / C followed
// mid authors (push) / D everyone else (explore) / E big V food (pull).
func showcaseClass(author uint) int {
	switch {
	case author == BigVTechID:
		return 0
	case author == BigVFoodID: // before readerFollows: the reader follows it, but it is pulled
		return 4
	case slices.Contains(interestAuthors, author):
		return 1
	case slices.Contains(readerFollows, author):
		return 2
	default:
		return 3
	}
}

type placedItem struct {
	topic  string
	author uint
	item   showcaseItem
}

// showcaseContent loads the reviewed JSON and assigns IDs from 10001 so the
// newest IDs rotate through classes A,B,C,D,E. Answers follow their question.
func (g *generator) showcaseContent() error {
	queues := make([][]placedItem, 5)
	for _, topic := range topicOrder {
		data, err := showcaseFS.ReadFile("showcase/" + topic + ".json")
		if err != nil {
			return err
		}
		var file showcaseFile
		if err := json.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("showcase %s: %w", topic, err)
		}
		for _, item := range file.Items {
			author, err := g.resolveAuthor(item.Author, topic)
			if err != nil {
				return err
			}
			c := showcaseClass(author)
			queues[c] = append(queues[c], placedItem{topic: topic, author: author, item: item})
		}
	}
	var newestFirst []placedItem
	for remaining := true; remaining; {
		remaining = false
		for c := range queues {
			if len(queues[c]) > 0 {
				newestFirst = append(newestFirst, queues[c][0])
				queues[c] = queues[c][1:]
				remaining = true
			}
		}
	}
	slices.Reverse(newestFirst)

	total := 0
	for _, p := range newestFirst {
		total += 1 + len(p.item.Answers)
	}
	from, span := g.now.AddDate(0, 0, -7), 7*24*time.Hour-10*time.Minute
	next, index := ShowcaseStartID, 0
	at := func() time.Time {
		index++
		return from.Add(span * time.Duration(index) / time.Duration(total+1))
	}
	var roots []uint
	for _, p := range newestFirst {
		root := next
		next++
		kind := model.ContentKind(p.item.Kind)
		if kind != model.ContentKindQuestion {
			kind = model.ContentKindArticle
		}
		g.addContent(model.Content{ID: root, AuthorID: p.author, Kind: kind, Topic: p.topic,
			Title: p.item.Title, Body: p.item.Body, CreatedAt: at()})
		g.eligible = append(g.eligible, root)
		roots = append(roots, root)
		if p.item.Hot {
			g.ds.HotContentID = root
		}
		for _, a := range p.item.Answers {
			author, err := g.resolveAuthor(a.Author, p.topic)
			if err != nil {
				return err
			}
			parent := root
			g.addContent(model.Content{ID: next, AuthorID: author, Kind: model.ContentKindAnswer, Topic: p.topic,
				ParentID: &parent, Body: a.Body, CreatedAt: at()})
			g.eligible = append(g.eligible, next)
			next++
		}
	}
	g.ds.ShowcaseCount = total

	commentID := CommentStartID
	for _, root := range roots {
		if g.r.Float64() >= 0.4 {
			continue
		}
		for k := 1 + g.r.IntN(3); k > 0; k-- {
			parent := root
			created := g.contentTime[root].Add(time.Duration(1+g.r.IntN(180)) * time.Minute)
			if created.After(g.now) {
				created = g.now
			}
			g.addContent(model.Content{ID: commentID, AuthorID: g.ordinary[g.r.IntN(len(g.ordinary))],
				Kind: model.ContentKindComment, Topic: g.contentTopic[root], ParentID: &parent,
				Body: commentBank[g.r.IntN(len(commentBank))], CreatedAt: created})
			commentID++
		}
	}
	return nil
}

// ------------------------------------------------------------ reactions --

func (g *generator) react(user, content uint, kind string, behavior model.BehaviorKind, duration int) bool {
	key := [2]uint{user, content}
	if g.reacted[key] == nil {
		g.reacted[key] = map[string]bool{}
	}
	if kind != "" && g.reacted[key][kind] {
		return false
	}
	ct := g.contentTime[content]
	t := ct.Add(time.Duration(g.r.Int64N(int64(g.now.Sub(ct)) + 1)))
	if kind != "" {
		g.reacted[key][kind] = true
		g.ds.Reactions = append(g.ds.Reactions, model.Reaction{UserID: user, ContentID: content, Kind: kind, CreatedAt: t})
	}
	g.ds.Behaviors = append(g.ds.Behaviors, model.BehaviorEvent{
		UserID: user, ContentID: content, Kind: behavior, Duration: duration, CreatedAt: t,
	})
	return true
}

// reactions draws likes and collections from a Zipf distribution over a
// popularity ranking, so a few posts collect most interactions.
func (g *generator) reactions() {
	pool := slices.Clone(g.eligible)
	for _, c := range g.ds.Contents {
		if c.ID < ShowcaseStartID && g.r.IntN(10) == 0 {
			pool = append(pool, c.ID)
		}
	}
	g.r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if hot := slices.Index(pool, g.ds.HotContentID); hot > 0 {
		pool[0], pool[hot] = pool[hot], pool[0]
	}
	zipf := rand.NewZipf(g.r, 1.1, 1, uint64(len(pool)-1))
	pick := func(user uint, want int, kind string, behavior model.BehaviorKind) {
		got := 0
		for tries := want * 10; tries > 0 && got < want; tries-- {
			id := pool[zipf.Uint64()]
			if g.contentAuthor[id] == user {
				continue
			}
			if !slices.Contains(g.prefs[user], g.contentTopic[id]) && g.r.Float64() > 0.3 {
				continue
			}
			if g.react(user, id, kind, behavior, 0) {
				got++
			}
		}
	}
	for _, user := range g.ordinary {
		pick(user, g.between(g.p.LikesPerUser), "like", model.BehaviorKindLike)
		pick(user, g.between(g.p.CollectsPerUser), "collect", model.BehaviorKindCollect)
	}
	if g.ds.HotContentID != 0 {
		likes := 0
		for _, r := range g.ds.Reactions {
			if r.ContentID == g.ds.HotContentID && r.Kind == "like" {
				likes++
			}
		}
		for _, user := range g.ordinary {
			if likes >= g.p.HotLikes {
				break
			}
			if g.react(user, g.ds.HotContentID, "like", model.BehaviorKindLike, 0) {
				likes++
			}
		}
	}
}

// readerBehavior gives the demo reader a fixed history: likes on unfollowed
// photo authors (interest stream), collections on followed tech authors and
// reading time on photo and fitness posts.
func (g *generator) readerBehavior() {
	byAuthors := func(authors []uint, topic string, n int) []uint {
		var ids []uint
		for i := len(g.eligible) - 1; i >= 0 && len(ids) < n; i-- {
			id := g.eligible[i]
			if g.contentTopic[id] != topic {
				continue
			}
			if authors == nil || slices.Contains(authors, g.contentAuthor[id]) {
				ids = append(ids, id)
			}
		}
		return ids
	}
	for _, id := range byAuthors(interestAuthors, model.TopicPhoto, 8) {
		g.react(ReaderID, id, "like", model.BehaviorKindLike, 0)
	}
	for _, id := range byAuthors([]uint{midID("tech", 1), midID("tech", 2)}, model.TopicTech, 2) {
		g.react(ReaderID, id, "collect", model.BehaviorKindCollect, 0)
	}
	for _, id := range byAuthors(nil, model.TopicPhoto, 6) {
		g.react(ReaderID, id, "", model.BehaviorKindReadTime, 60+g.r.IntN(240))
	}
	for _, id := range byAuthors(nil, model.TopicFitness, 4) {
		g.react(ReaderID, id, "", model.BehaviorKindView, 0)
	}
}

// onlineUsers picks ordinary users that follow at least one push-mode author;
// their inboxes are pre-built so an ordinary author's post visibly reaches
// active followers.
func (g *generator) onlineUsers() {
	candidates := slices.Clone(g.ordinary)
	g.r.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	for _, id := range candidates {
		if len(g.ds.OnlineUsers) >= g.p.Warm {
			break
		}
		for _, f := range g.followingOf[id] {
			if f >= MidStartID {
				g.ds.OnlineUsers = append(g.ds.OnlineUsers, id)
				break
			}
		}
	}
	slices.Sort(g.ds.OnlineUsers)
}
