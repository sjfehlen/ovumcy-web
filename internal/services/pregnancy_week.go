package services

import "strconv"

// PregnancyWeekInfo is the fixed reference entry for one gestational week: the
// everyday-object size comparison, the approximate length, and the key of one
// short development line. Every key is spelled out as a literal so the locale
// reachability sweep can see it.
//
// Length is crown-to-rump up to week 19 and crown-to-heel from week 20, the
// convention the common size tables use. Both are rough averages, which the card
// says next to them.
type PregnancyWeekInfo struct {
	Week     int
	FruitID  string
	FruitKey string
	LengthCM float64
	DevKey   string
}

const (
	pregnancyFirstSizedWeek = 4
	pregnancyLastSizedWeek  = 42
)

var pregnancyWeekTable = [...]PregnancyWeekInfo{
	{Week: 4, FruitID: "poppy_seed", FruitKey: "pregnancy.fruit.poppy_seed", LengthCM: 0.1, DevKey: "pregnancy.week.4.dev"},
	{Week: 5, FruitID: "sesame_seed", FruitKey: "pregnancy.fruit.sesame_seed", LengthCM: 0.2, DevKey: "pregnancy.week.5.dev"},
	{Week: 6, FruitID: "lentil", FruitKey: "pregnancy.fruit.lentil", LengthCM: 0.6, DevKey: "pregnancy.week.6.dev"},
	{Week: 7, FruitID: "blueberry", FruitKey: "pregnancy.fruit.blueberry", LengthCM: 1.3, DevKey: "pregnancy.week.7.dev"},
	{Week: 8, FruitID: "raspberry", FruitKey: "pregnancy.fruit.raspberry", LengthCM: 1.6, DevKey: "pregnancy.week.8.dev"},
	{Week: 9, FruitID: "cherry", FruitKey: "pregnancy.fruit.cherry", LengthCM: 2.3, DevKey: "pregnancy.week.9.dev"},
	{Week: 10, FruitID: "strawberry", FruitKey: "pregnancy.fruit.strawberry", LengthCM: 3.1, DevKey: "pregnancy.week.10.dev"},
	{Week: 11, FruitID: "fig", FruitKey: "pregnancy.fruit.fig", LengthCM: 4.1, DevKey: "pregnancy.week.11.dev"},
	{Week: 12, FruitID: "lime", FruitKey: "pregnancy.fruit.lime", LengthCM: 5.4, DevKey: "pregnancy.week.12.dev"},
	{Week: 13, FruitID: "lemon", FruitKey: "pregnancy.fruit.lemon", LengthCM: 7.4, DevKey: "pregnancy.week.13.dev"},
	{Week: 14, FruitID: "peach", FruitKey: "pregnancy.fruit.peach", LengthCM: 8.7, DevKey: "pregnancy.week.14.dev"},
	{Week: 15, FruitID: "apple", FruitKey: "pregnancy.fruit.apple", LengthCM: 10.1, DevKey: "pregnancy.week.15.dev"},
	{Week: 16, FruitID: "avocado", FruitKey: "pregnancy.fruit.avocado", LengthCM: 11.6, DevKey: "pregnancy.week.16.dev"},
	{Week: 17, FruitID: "pear", FruitKey: "pregnancy.fruit.pear", LengthCM: 13, DevKey: "pregnancy.week.17.dev"},
	{Week: 18, FruitID: "bell_pepper", FruitKey: "pregnancy.fruit.bell_pepper", LengthCM: 14.2, DevKey: "pregnancy.week.18.dev"},
	{Week: 19, FruitID: "mango", FruitKey: "pregnancy.fruit.mango", LengthCM: 15.3, DevKey: "pregnancy.week.19.dev"},
	{Week: 20, FruitID: "banana", FruitKey: "pregnancy.fruit.banana", LengthCM: 25.6, DevKey: "pregnancy.week.20.dev"},
	{Week: 21, FruitID: "carrot", FruitKey: "pregnancy.fruit.carrot", LengthCM: 26.7, DevKey: "pregnancy.week.21.dev"},
	{Week: 22, FruitID: "papaya", FruitKey: "pregnancy.fruit.papaya", LengthCM: 27.8, DevKey: "pregnancy.week.22.dev"},
	{Week: 23, FruitID: "grapefruit", FruitKey: "pregnancy.fruit.grapefruit", LengthCM: 28.9, DevKey: "pregnancy.week.23.dev"},
	{Week: 24, FruitID: "corn", FruitKey: "pregnancy.fruit.corn", LengthCM: 30, DevKey: "pregnancy.week.24.dev"},
	{Week: 25, FruitID: "cauliflower", FruitKey: "pregnancy.fruit.cauliflower", LengthCM: 34.6, DevKey: "pregnancy.week.25.dev"},
	{Week: 26, FruitID: "lettuce", FruitKey: "pregnancy.fruit.lettuce", LengthCM: 35.6, DevKey: "pregnancy.week.26.dev"},
	{Week: 27, FruitID: "broccoli", FruitKey: "pregnancy.fruit.broccoli", LengthCM: 36.6, DevKey: "pregnancy.week.27.dev"},
	{Week: 28, FruitID: "eggplant", FruitKey: "pregnancy.fruit.eggplant", LengthCM: 37.6, DevKey: "pregnancy.week.28.dev"},
	{Week: 29, FruitID: "butternut_squash", FruitKey: "pregnancy.fruit.butternut_squash", LengthCM: 38.6, DevKey: "pregnancy.week.29.dev"},
	{Week: 30, FruitID: "cabbage", FruitKey: "pregnancy.fruit.cabbage", LengthCM: 39.9, DevKey: "pregnancy.week.30.dev"},
	{Week: 31, FruitID: "coconut", FruitKey: "pregnancy.fruit.coconut", LengthCM: 41.1, DevKey: "pregnancy.week.31.dev"},
	{Week: 32, FruitID: "spaghetti_squash", FruitKey: "pregnancy.fruit.spaghetti_squash", LengthCM: 42.4, DevKey: "pregnancy.week.32.dev"},
	{Week: 33, FruitID: "pineapple", FruitKey: "pregnancy.fruit.pineapple", LengthCM: 43.7, DevKey: "pregnancy.week.33.dev"},
	{Week: 34, FruitID: "cantaloupe", FruitKey: "pregnancy.fruit.cantaloupe", LengthCM: 45, DevKey: "pregnancy.week.34.dev"},
	{Week: 35, FruitID: "honeydew", FruitKey: "pregnancy.fruit.honeydew", LengthCM: 46.2, DevKey: "pregnancy.week.35.dev"},
	{Week: 36, FruitID: "romaine", FruitKey: "pregnancy.fruit.romaine", LengthCM: 47.4, DevKey: "pregnancy.week.36.dev"},
	{Week: 37, FruitID: "chard", FruitKey: "pregnancy.fruit.chard", LengthCM: 48.6, DevKey: "pregnancy.week.37.dev"},
	{Week: 38, FruitID: "leek", FruitKey: "pregnancy.fruit.leek", LengthCM: 49.8, DevKey: "pregnancy.week.38.dev"},
	{Week: 39, FruitID: "mini_watermelon", FruitKey: "pregnancy.fruit.mini_watermelon", LengthCM: 50.7, DevKey: "pregnancy.week.39.dev"},
	{Week: 40, FruitID: "pumpkin", FruitKey: "pregnancy.fruit.pumpkin", LengthCM: 51.2, DevKey: "pregnancy.week.40.dev"},
	{Week: 41, FruitID: "watermelon", FruitKey: "pregnancy.fruit.watermelon", LengthCM: 51.7, DevKey: "pregnancy.week.41.dev"},
	{Week: 42, FruitID: "watermelon", FruitKey: "pregnancy.fruit.watermelon", LengthCM: 52, DevKey: "pregnancy.week.42.dev"},
}

// PregnancyWeekInfoFor returns the reference entry for a completed gestational
// week. Before week 4 there is nothing to compare, so it reports false; a
// pregnancy past week 42 keeps the last entry.
func PregnancyWeekInfoFor(week int) (PregnancyWeekInfo, bool) {
	if week < pregnancyFirstSizedWeek {
		return PregnancyWeekInfo{}, false
	}
	if week > pregnancyLastSizedWeek {
		week = pregnancyLastSizedWeek
	}
	return pregnancyWeekTable[week-pregnancyFirstSizedWeek], true
}

// pregnancyMilestones are the checks usually scheduled in a week range. They are
// general timing hints, never a schedule: the card says timing varies.
var pregnancyMilestones = [...]struct {
	from, to int
	key      string
}{
	{10, 14, "pregnancy.milestone.first_trimester_screen"},
	{18, 22, "pregnancy.milestone.anatomy_scan"},
	{24, 28, "pregnancy.milestone.glucose"},
	{35, 37, "pregnancy.milestone.gbs"},
	{36, 42, "pregnancy.milestone.birth_prep"},
}

// PregnancyMilestoneKeys returns the message keys of the checks usually done
// around the given completed week.
func PregnancyMilestoneKeys(week int) []string {
	keys := []string{}
	for _, milestone := range pregnancyMilestones {
		if week >= milestone.from && week <= milestone.to {
			keys = append(keys, milestone.key)
		}
	}
	return keys
}

// PregnancyLength is a size ready to be localized: the unit key and the number
// already spelled with the language's decimal separator.
type PregnancyLength struct {
	UnitKey string
	Value   string
}

// FormatPregnancyLength spells a length in millimetres below one centimetre and
// in centimetres with one decimal otherwise. Every shipped language except
// English writes the decimal comma.
func FormatPregnancyLength(cm float64, language string) PregnancyLength {
	separator := ","
	if language == "en" {
		separator = "."
	}
	if cm < 1 {
		return PregnancyLength{UnitKey: "pregnancy.length_mm", Value: formatDecimal(cm*10, separator)}
	}
	return PregnancyLength{UnitKey: "pregnancy.length_cm", Value: formatDecimal(cm, separator)}
}

func formatDecimal(value float64, separator string) string {
	whole := int(value)
	tenths := int(value*10+0.5) - whole*10
	if tenths >= 10 {
		whole++
		tenths = 0
	}
	if tenths == 0 {
		return strconv.Itoa(whole)
	}
	return strconv.Itoa(whole) + separator + strconv.Itoa(tenths)
}

// PregnancyWeekView is what the dashboard's "this week" card renders: the size
// comparison, the length, one development line and the checks usually done
// around now. Visible is false before the first sized week.
type PregnancyWeekView struct {
	Visible       bool
	Week          int
	FruitKey      string
	Length        PregnancyLength
	DevKey        string
	MilestoneKeys []string
}

// BuildPregnancyWeekView derives the card from the pregnancy status. It adds no
// stored data: the week comes from the due date and the entry from the table.
func BuildPregnancyWeekView(status PregnancyStatus, language string) PregnancyWeekView {
	if !status.Active {
		return PregnancyWeekView{}
	}
	info, ok := PregnancyWeekInfoFor(status.Weeks)
	if !ok {
		return PregnancyWeekView{}
	}
	return PregnancyWeekView{
		Visible:       true,
		Week:          status.Weeks,
		FruitKey:      info.FruitKey,
		Length:        FormatPregnancyLength(info.LengthCM, language),
		DevKey:        info.DevKey,
		MilestoneKeys: PregnancyMilestoneKeys(status.Weeks),
	}
}
