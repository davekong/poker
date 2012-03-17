// Functions and structures for calculating the equity of poker hands.
// The hand evaluation code is based on
// http://www.codingthewheel.com/archives/poker-hand-evaluator-roundup#2p2
//
//	Example hands that can be parsed
//	*********************************
//	String  Combinations  Description
//
//	AJs                4  Any Ace with a Jack of the same suit.
//	77                 6  Any pair of Sevens.
//	T9o               12  Any Ten and Nine of different suits.
//	54                16  Any Five and Four, suited or unsuited.
//
//	AJs+              12  Any Ace with a (Jack through King) of the same suit.
//	77+               48  Any pair greater than or equal to Sevens.
//	T9o-65o           12  Any unsuited connector between 65o and T9o.
//
//	QQ+,AQs+,AK       38  Any pair of Queen or better, any AQs, and any AK
//	                      whether suited or not.
//	AhKh,7h7d          2  Ace-King of Hearts or a pair of red Sevens.
package equity

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"io"
	"runtime"

	"poker/comb"
)

const (
	ranks = "23456789TJQKA"
	suits = "cdhs"
)

var hr [32487834]uint32
var CTOI map[string]uint32
var NCPU int // How many cpus to use for the equity calculations.

func init() {
	fmt.Print("Loading HandRanks.dat... ")
	// Initialize hr
	buf := make([]byte, len(hr)*4, len(hr)*4)
	fp, err := os.Open("HandRanks.dat")
	if err != nil {
		panic(err)
	}
	defer fp.Close()
	_, err = io.ReadFull(fp, buf)
	if err != nil {
		panic(err)
	}
	for i := 0; i < len(buf); i += 4 {
		hr[i/4] = uint32(buf[i+3])<<24 |
			uint32(buf[i+2])<<16 |
			uint32(buf[i+1])<<8 |
			uint32(buf[i])
	}
	fmt.Println("Done")

	// Initialize CTOI
	CTOI = make(map[string]uint32, 52)
	var k uint32 = 1
	for i := 0; i < len(ranks); i++ {
		for j := 0; j < len(suits); j++ {
			CTOI[string([]byte{ranks[i], suits[j]})] = k
			k++
		}
	}

	NCPU = runtime.NumCPU()
	// FIXME: Increasing the number of CPUs slows the program down and makes the
	// outcomes non-deterministic.
	//runtime.GOMAXPROCS(NCPU)
	//fmt.Printf("Using %d CPUs\n", NCPU)
}

func NewDeck(missing ...uint32) []uint32 {
	deck := make([]uint32, 52, 52)
	for i := 0; i < 52; i++ {
		deck[i] = uint32(i + 1)
	}
	if len(missing) > 0 {
		deck = minus(deck, missing)
	}
	return deck
}

func cardsToInts(cards []string) []uint32 {
	ints := make([]uint32, len(cards), len(cards))
	for i, c := range cards {
		ints[i] = CTOI[c]
	}
	return ints
}

// A hand distribution is a category of hands. Currently the only categories
// supported are those of the forms AA, AKo, and AKs.
type HandDist struct {
	Dist string
}

// Expand the HandDist into a slice of all possible hands.
func (this *HandDist) Strs() [][]string {
	hands := make([][]string, 0)
	// Expand each card into a card of each suit
	xs := make([]string, 4)
	ys := make([]string, 4)
	for i := range suits {
		xs[i] = string([]byte{this.Dist[0], suits[i]})
		ys[i] = string([]byte{this.Dist[1], suits[i]})
	}
	switch {
	case len(this.Dist) == 2:
		// pairs e.g. AA
		for i := 0; i < 3; i++ {
			for j := i+1; j < 4; j++ {
				hands = append(hands, []string{xs[i], xs[j]})
			}
		}
	case this.Dist[2] == 'o':
		// offsuit e.g. AKo
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				if i != j {
					hands = append(hands, []string{xs[i], ys[j]})
				}
			}
		}
	default:
		// suited e. g. AKs
		for i := 0; i < 4; i++ {
			hands = append(hands, []string{xs[i], ys[i]})
		}
	}
	return hands
}

// The same as Strs, only return the hands represented by uint32s.
func (this *HandDist) Ints() [][]uint32 {
	shands := this.Strs()
	hands := make([][]uint32, len(shands))
	for i := range shands {
		hands[i] = cardsToInts(shands[i])
	}
	return hands
}

// Create a HandDist from rank, rank, suit int values.
func NewRRSDist(r1, r2, suit int) *HandDist {
	hand := []byte{ranks[r1-2], ranks[r2-2]}
	switch {
	case r1 == r2:
		return &HandDist{string(hand)}              // pair
	case suit > 0:
		return &HandDist{string(append(hand, 's'))} // suited
	}
	return &HandDist{string(append(hand, 'o'))}     // offsuit
}

func evalBoard(cards []uint32) uint32 {
	v := hr[53+cards[0]]
	v = hr[v+cards[1]]
	v = hr[v+cards[2]]
	v = hr[v+cards[3]]
	return hr[v+cards[4]]
}

func evalHand(b uint32, cards []uint32) uint32 {
	b = hr[b+cards[0]]
	return hr[b+cards[1]]
}

func EvalHand(cards []string) uint32 {
	hand := cardsToInts(cards)
	v := hr[53+hand[0]]
	v = hr[v+hand[1]]
	v = hr[v+hand[2]]
	v = hr[v+hand[3]]
	v = hr[v+hand[4]]
	v = hr[v+hand[5]]
	return hr[v+hand[6]]
}


// Split a hand rank into two values: category and rank-within-category.
func SplitRank(rank uint32) (uint32, uint32) {
	return rank >> 12, rank & 0xFFF
}

// Calculate the percent of the pot each hand wins and return them as a slice.
func evalHands(board []uint32, hands ...[]uint32) []float64 {
	b := evalBoard(board)
	// Optimize case where there are only two hands.
	if len(hands) == 2 {
		result := evalHand(b, hands[0]) - evalHand(b, hands[1])
		switch {
		case result > 0:
			return []float64{1, 0}
		case result < 0:
			return []float64{0, 1}
		default:
			return []float64{0.5, 0.5}
		}
	}
	vals := make([]uint32, len(hands), len(hands))
	for i, hand := range hands {
		vals[i] = evalHand(b, hand)
	}
	// Determine the number of winners and their hand.
	winners := 1
	max := vals[0]
	for i := 1; i < len(vals); i++ {
		if v := vals[i]; v > max {
			max = v
			winners = 1
		} else if v == max {
			winners++
		}
	}
	// Alot each winner his share of the pot.
	result := make([]float64, len(hands), len(hands))
	for i, v := range vals {
		if v == max {
			result[i] = 1.0 / float64(winners)
		} else {
			result[i] = 0.0
		}
	}
	return result
}

// Calculate the probability of having a given class of hole cards.
func PHole(scards []string, hd *HandDist) float64 {
	// Remove seen cards from the deck.
	cards := cardsToInts(scards)
	deck := NewDeck(cards...)

	// calculate the ratio dist : all-hands.
	// FIXME Might want to work in big.Int instead of converting back to uint. Could
	// there be overflow here?
	combs := uint64(comb.Count(big.NewInt(int64(len(deck))), comb.TWO).Int64())
	allHands := make([][]uint32, combs)
	c := comb.Generator(deck, 2)
	hand := make([]uint32, 2)
	for i := uint64(0); i < combs; i++ {
		c(hand)
		copy(allHands[i], hand)
	}
	hands := hd.Ints()
    return float64(len(intersect(hands, allHands))) / float64(len(allHands))
}


// The formula for calculating the conditional probability P(hole | action):
//
//	                   P(hole) * P(action | hole)
//	P(hole | action) = --------------------------
//	                            P(action)
//
// Weisstein, Eric W. "Conditional Probability." From MathWorld--A Wolfram Web
// Resource. http://mathworld.wolfram.com/ConditionalProbability.html
//
// P(hole) is calculated by dividing the number of hands included in a
// class by the total possible number of hands. Cards that have been seen are
// eliminated from the possible hands.
// 	          Me   Opp  Board  P(AA)
//	Pre-deal  ??   ??   ???    (4 choose 2) / (52 choose 2) ~= 0.0045
//  Pre-flop  AKs  ??   ???    (3 choose 2) / (50 choose 2) ~= 0.0024

// Given a game's card string and the conditional probabilities P(action | hole),
// calculates the probabilities P(hole | action).
//func CondProbs(cards []string HandDist) map[string] string {
//	for _, vals := range actionDist {
//		NewRRSDist(actionDist[:3]...) (* (PHole cards [r1 r2 s]) prob)])]
//  (apply array-map (flatten values))))
// FIXME

type Lottery struct {
	// Maybe should use ints or fixed point to make more accurate.
	probs []float64
	prizes []string
}

func (this *Lottery) String() string {
	b := bytes.NewBufferString("[ ")
	for i := 0; i < len(this.probs); i++ {
		fmt.Fprintf(b, "%s:%.2f ", this.prizes[i], this.probs[i])
	}
	b.WriteString("]")
	return b.String()
}

// Convert a discrete distribution (array-map {item prob}) into a lottery. The
// probabilities should add up to 1
func NewLottery(dist map[string] float64) *Lottery {
	sum := 0.0
	lotto := &Lottery{}
	for key, val := range dist {
		if val != 0 {
			sum += val
			lotto.probs = append(lotto.probs, sum)
			lotto.prizes = append(lotto.prizes, key)
		}
	}
	return lotto
}

// Draw a winner from a Lottery. If at least one value in the lottery is not >=
// 1, then the greatest value is effectively rounded up to 1.0"
func (this *Lottery) Play() string {
	draw := rand.Float64()
	for i, p := range this.probs {
		if p > draw {
			return this.prizes[i]
		}
	}
	return this.prizes[len(this.prizes)-1]
}

// Safe subtraction of integer sets.
func minus(a, b []uint32) []uint32 {
	c := make([]uint32, len(a), len(a))
	var count int
	var match bool
	for _, v := range a {
		for _, w := range b {
			if v == w {
				match = true
				break
			}
		}
		if !match {
			c[count] = v
			count++
		}
		match = false
	}
	return c[:count]
}

// intersect returns the intersection of the sets a and b.
// len(a) <= len(b) should be true for best performance.
func intersect(a, b [][]uint32) [][]uint32 {
	c := make([][]uint32, len(a), len(a))
	var count int
	for _, v := range a {
		for _, w := range b {
			if (v[0] == w[0] && v[1] == w[1]) || (v[0] == w[1] && v[1] == w[0]) {
				c[count] = v
				count++
				break
			}
		}
	}
	return c[:count]
}

func shuffle(a []uint32) {
	for i := len(a) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		a[j], a[i] = a[i], a[j]
	}
}

// HandEquity returns the equity of a player's hand based on the current
// board.  trials is the number of Monte-Carlo simulations to do.  If trials
// is 0, then exhaustive enumeration will be used instead.
func HandEquity(sHand, sBoard []string, trials int, c chan float64) {
	sum := 0.0
	// Convert the cards from strings to ints.
	hole := cardsToInts(sHand)
	bLen := uint32(len(sBoard)) // How many cards will we need to draw?
	board := make([]uint32, 5, 5)
	for i, v := range sBoard {
		board[i] = CTOI[v]
	}

	// Remove the hole and board cards from the deck.
	deck := NewDeck(append(hole, board...)...)

	if trials == 0 {
		var count float64
		// Exhaustive enumeration.
		oHole := make([]uint32, 2, 2)
		loop1, loop2 := true, true
		c1 := comb.Generator(deck, 2)
		for loop1 {
			loop1 = c1(oHole)
			c2 := comb.Generator(minus(deck, oHole), 5-bLen)
			for loop2 {
				loop2 = c2(board[bLen:])
				sum += evalHands(board, hole, oHole)[0]
				count++
			}
		}
		c <- sum / count
	} else {
		// Monte-Carlo
		for i := 0; i < trials; i++ {
			shuffle(deck)
			copy(board[bLen:], deck[2:8-bLen])
			sum += evalHands(board, hole, deck[:2])[0]
		}
		c <- sum / float64(trials)
	}
}

// Parallel version of HandEquity.
func HandEquity2(sHand, sBoard []string, trials int) float64 {
	sum := 0.0
	trials += trials % NCPU // Round to a multiple of the number of CPUs.
    c := make(chan float64) // Not buffering
    for i := 0; i < NCPU; i++ {
        go HandEquity(sHand, sBoard, trials/NCPU, c)
    }
    for i := 0; i < NCPU; i++ {
        sum += <-c
    }
	return sum / float64(NCPU)
}
