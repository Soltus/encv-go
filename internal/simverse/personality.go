package simverse

import "math/rand"

// ============================================================
// 个性系统 — 大五人格 (OCEAN) + 价值观 + 兴趣爱好
//
// 设计原则：
//   - 用 uint8 存储（0-100 百分位），省内存
//   - 生成时基于物种/性别/职业倾向做偏置
//   - 静态数据（生成后基本不变），不占 tick 计算资源
//   - 影响事件概率、交互结果、职业发展方向
// ============================================================

type BigFiveTrait uint8

const (
	TraitOpenness     BigFiveTrait = 0
	TraitConscientious BigFiveTrait = 1
	TraitExtraversion BigFiveTrait = 2
	TraitAgreeableness BigFiveTrait = 3
	TraitNeuroticism  BigFiveTrait = 4
	BigFiveMax                     = 5
)

var BigFiveNames = [BigFiveMax]string{
	"Openness",
	"Conscientiousness",
	"Extraversion",
	"Agreeableness",
	"Neuroticism",
}

var BigFiveCNNames = [BigFiveMax]string{
	"开放性",
	"尽责性",
	"外向性",
	"宜人性",
	"神经质",
}

func (t BigFiveTrait) String() string {
	if int(t) < int(BigFiveMax) {
		return BigFiveNames[t]
	}
	return "Unknown"
}

func (t BigFiveTrait) CN() string {
	if int(t) < int(BigFiveMax) {
		return BigFiveCNNames[t]
	}
	return "未知"
}

type BigFive [BigFiveMax]uint8

func (b BigFive) Get(trait BigFiveTrait) uint8 {
	if int(trait) < int(BigFiveMax) {
		return b[trait]
	}
	return 50
}

func (b BigFive) IsHigh(trait BigFiveTrait) bool {
	return b.Get(trait) >= 70
}

func (b BigFive) IsLow(trait BigFiveTrait) bool {
	return b.Get(trait) <= 30
}

type ValueType uint8

const (
	ValueAchievement   ValueType = 0
	ValueBenevolence   ValueType = 1
	ValueConformity    ValueType = 2
	ValueHedonism      ValueType = 3
	ValuePower         ValueType = 4
	ValueSecurity      ValueType = 5
	ValueSelfDirection ValueType = 6
	ValueStimulation   ValueType = 7
	ValueTradition     ValueType = 8
	ValueUniversalism  ValueType = 9
	ValueMax                      = 10
)

var ValueNames = [ValueMax]string{
	"Achievement",
	"Benevolence",
	"Conformity",
	"Hedonism",
	"Power",
	"Security",
	"SelfDirection",
	"Stimulation",
	"Tradition",
	"Universalism",
}

var ValueCNNames = [ValueMax]string{
	"成就",
	"仁慈",
	"遵从",
	"享乐",
	"权力",
	"安全",
	"自我导向",
	"刺激",
	"传统",
	"普世性",
}

func (v ValueType) String() string {
	if int(v) < int(ValueMax) {
		return ValueNames[v]
	}
	return "Unknown"
}

func (v ValueType) CN() string {
	if int(v) < int(ValueMax) {
		return ValueCNNames[v]
	}
	return "未知"
}

type Values [ValueMax]uint8

func (v Values) Get(val ValueType) uint8 {
	if int(val) < int(ValueMax) {
		return v[val]
	}
	return 50
}

func (v Values) Top3() []ValueType {
	type pair struct {
		t ValueType
		v uint8
	}
	pairs := make([]pair, ValueMax)
	for i := 0; i < int(ValueMax); i++ {
		pairs[i] = pair{ValueType(i), v[i]}
	}
	for i := 0; i < 3; i++ {
		maxIdx := i
		for j := i + 1; j < int(ValueMax); j++ {
			if pairs[j].v > pairs[maxIdx].v {
				maxIdx = j
			}
		}
		pairs[i], pairs[maxIdx] = pairs[maxIdx], pairs[i]
	}
	result := make([]ValueType, 3)
	for i := 0; i < 3; i++ {
		result[i] = pairs[i].t
	}
	return result
}

type InterestType uint8

const (
	InterestArt        InterestType = 0
	InterestSports     InterestType = 1
	InterestReading    InterestType = 2
	InterestSocial     InterestType = 3
	InterestNature     InterestType = 4
	InterestTechnology InterestType = 5
	InterestCooking    InterestType = 6
	InterestMusic      InterestType = 7
	InterestTravel     InterestType = 8
	InterestGambling   InterestType = 9
	InterestCraft      InterestType = 10
	InterestMedicine   InterestType = 11
	InterestMax                     = 12
)

var InterestNames = [InterestMax]string{
	"Art",
	"Sports",
	"Reading",
	"Social",
	"Nature",
	"Technology",
	"Cooking",
	"Music",
	"Travel",
	"Gambling",
	"Craft",
	"Medicine",
}

var InterestCNNames = [InterestMax]string{
	"艺术",
	"运动",
	"阅读",
	"社交",
	"自然",
	"科技",
	"烹饪",
	"音乐",
	"旅行",
	"赌博",
	"手工",
	"医药",
}

func (i InterestType) String() string {
	if int(i) < int(InterestMax) {
		return InterestNames[i]
	}
	return "Unknown"
}

func (i InterestType) CN() string {
	if int(i) < int(InterestMax) {
		return InterestCNNames[i]
	}
	return "未知"
}

type Interests [InterestMax]uint8

func (in Interests) Get(interest InterestType) uint8 {
	if int(interest) < int(InterestMax) {
		return in[interest]
	}
	return 0
}

func (in Interests) TopN(n int) []InterestType {
	if n <= 0 || n > int(InterestMax) {
		n = 3
	}
	type pair struct {
		t InterestType
		v uint8
	}
	pairs := make([]pair, InterestMax)
	for i := 0; i < int(InterestMax); i++ {
		pairs[i] = pair{InterestType(i), in[i]}
	}
	for i := 0; i < n; i++ {
		maxIdx := i
		for j := i + 1; j < int(InterestMax); j++ {
			if pairs[j].v > pairs[maxIdx].v {
				maxIdx = j
			}
		}
		pairs[i], pairs[maxIdx] = pairs[maxIdx], pairs[i]
	}
	result := make([]InterestType, n)
	for i := 0; i < n; i++ {
		result[i] = pairs[i].t
	}
	return result
}

func (in Interests) Has(interest InterestType) bool {
	return in.Get(interest) >= 40
}

type Personality struct {
	BigFive   BigFive
	Values    Values
	Interests Interests
}

func PersonalitySize() float64 {
	return float64(BigFiveMax + ValueMax + InterestMax)
}

func GeneratePersonality(rng *rand.Rand, species SpeciesType, profession ProfessionType, gender Gender) Personality {
	var p Personality

	for i := 0; i < int(BigFiveMax); i++ {
		p.BigFive[i] = uint8(clampInt(int(randNormal(rng, 50, 20)), 5, 95))
	}

	applyProfessionPersonalityBias(&p.BigFive, profession)
	applySpeciesPersonalityBias(&p.BigFive, species)

	for i := 0; i < int(ValueMax); i++ {
		p.Values[i] = uint8(clampInt(int(randNormal(rng, 50, 20)), 5, 95))
	}
	deriveValuesFromBigFive(&p.Values, &p.BigFive)

	for i := 0; i < int(InterestMax); i++ {
		p.Interests[i] = uint8(clampInt(int(randNormal(rng, 30, 25)), 0, 95))
	}
	deriveInterestsFromBigFive(&p.Interests, &p.BigFive)
	applyProfessionInterestBias(&p.Interests, profession)

	return p
}

func applyProfessionPersonalityBias(bf *BigFive, prof ProfessionType) {
	bias := getProfessionPersonalityBias(prof)
	for i := 0; i < int(BigFiveMax); i++ {
		bf[i] = uint8(clampInt(int(bf[i])+bias[i], 5, 95))
	}
}

func getProfessionPersonalityBias(prof ProfessionType) [BigFiveMax]int {
	switch prof {
	case ProfWarrior:
		return [BigFiveMax]int{0, 10, 10, -5, -10}
	case ProfMage:
		return [BigFiveMax]int{15, 10, -10, 0, 5}
	case ProfRogue:
		return [BigFiveMax]int{5, 10, 5, -10, 0}
	case ProfPriest:
		return [BigFiveMax]int{0, 10, -5, 15, -5}
	case ProfMerchant:
		return [BigFiveMax]int{5, 5, 10, 5, 0}
	case ProfFarmer:
		return [BigFiveMax]int{-10, 15, -5, 10, -10}
	case ProfBlacksmith:
		return [BigFiveMax]int{0, 15, -5, 0, -5}
	case ProfScribe:
		return [BigFiveMax]int{20, 10, -10, 5, 5}
	case ProfNoble:
		return [BigFiveMax]int{5, 5, 15, 0, 0}
	case ProfEntertainer:
		return [BigFiveMax]int{15, -5, 20, 5, 5}
	case ProfMiner:
		return [BigFiveMax]int{-5, 15, -10, 0, 5}
	case ProfWoodcutter:
		return [BigFiveMax]int{-5, 10, 0, 5, -5}
	case ProfTailor:
		return [BigFiveMax]int{5, 15, -5, 5, 0}
	case ProfRanger:
		return [BigFiveMax]int{5, 10, 5, 0, 0}
	case ProfAlchemist:
		return [BigFiveMax]int{15, 15, -5, 0, 5}
	case ProfCook:
		return [BigFiveMax]int{0, 10, 5, 10, -5}
	case ProfHealer:
		return [BigFiveMax]int{5, 10, -5, 15, 0}
	case ProfSlave, ProfBeggar:
		return [BigFiveMax]int{-10, -5, -10, 0, 15}
	default:
		return [BigFiveMax]int{}
	}
}

func applySpeciesPersonalityBias(bf *BigFive, species SpeciesType) {
	bias := getSpeciesPersonalityBias(species)
	for i := 0; i < int(BigFiveMax); i++ {
		bf[i] = uint8(clampInt(int(bf[i])+bias[i], 5, 95))
	}
}

func getSpeciesPersonalityBias(species SpeciesType) [BigFiveMax]int {
	switch species {
	case SpeciesHuman:
		return [BigFiveMax]int{0, 0, 0, 0, 0}
	case SpeciesElf:
		return [BigFiveMax]int{10, 5, -5, 5, 0}
	case SpeciesDwarf:
		return [BigFiveMax]int{-5, 15, -10, 0, -5}
	case SpeciesOrc:
		return [BigFiveMax]int{-10, -5, 10, -10, 10}
	case SpeciesBeastman:
		return [BigFiveMax]int{-5, 5, 10, -5, 5}
	case SpeciesDragonkin:
		return [BigFiveMax]int{5, 15, 5, -10, -5}
	case SpeciesFey:
		return [BigFiveMax]int{15, 0, 10, 5, 5}
	case SpeciesUndead:
		return [BigFiveMax]int{5, 10, -15, -10, 10}
	default:
		return [BigFiveMax]int{}
	}
}

func deriveValuesFromBigFive(v *Values, bf *BigFive) {
	openness := int(bf[TraitOpenness])
	conscientious := int(bf[TraitConscientious])
	extraversion := int(bf[TraitExtraversion])
	agreeableness := int(bf[TraitAgreeableness])
	neuroticism := int(bf[TraitNeuroticism])

	v[ValueAchievement] = uint8(clampInt(int(v[ValueAchievement])+(conscientious-50)/2, 5, 95))
	v[ValuePower] = uint8(clampInt(int(v[ValuePower])+(extraversion-50)/3+(100-agreeableness)/3, 5, 95))
	v[ValueBenevolence] = uint8(clampInt(int(v[ValueBenevolence])+(agreeableness-50)/2, 5, 95))
	v[ValueUniversalism] = uint8(clampInt(int(v[ValueUniversalism])+(openness-50)/2+(agreeableness-50)/3, 5, 95))
	v[ValueSelfDirection] = uint8(clampInt(int(v[ValueSelfDirection])+(openness-50)/2, 5, 95))
	v[ValueStimulation] = uint8(clampInt(int(v[ValueStimulation])+(openness-50)/2+(extraversion-50)/3, 5, 95))
	v[ValueHedonism] = uint8(clampInt(int(v[ValueHedonism])+(extraversion-50)/3+(neuroticism-50)/4, 5, 95))
	v[ValueSecurity] = uint8(clampInt(int(v[ValueSecurity])+(100-neuroticism)/3+(conscientious-50)/3, 5, 95))
	v[ValueConformity] = uint8(clampInt(int(v[ValueConformity])+(conscientious-50)/2+(100-openness)/3, 5, 95))
	v[ValueTradition] = uint8(clampInt(int(v[ValueTradition])+(100-openness)/2+(conscientious-50)/3, 5, 95))
}

func deriveInterestsFromBigFive(in *Interests, bf *BigFive) {
	openness := int(bf[TraitOpenness])
	extraversion := int(bf[TraitExtraversion])
	agreeableness := int(bf[TraitAgreeableness])
	conscientious := int(bf[TraitConscientious])

	in[InterestArt] = uint8(clampInt(int(in[InterestArt])+(openness-50)/2, 0, 95))
	in[InterestReading] = uint8(clampInt(int(in[InterestReading])+(openness-50)/3+(100-extraversion)/3, 0, 95))
	in[InterestMusic] = uint8(clampInt(int(in[InterestMusic])+(openness-50)/3+(extraversion-50)/4, 0, 95))
	in[InterestSocial] = uint8(clampInt(int(in[InterestSocial])+(extraversion-50)/2+(agreeableness-50)/4, 0, 95))
	in[InterestSports] = uint8(clampInt(int(in[InterestSports])+(extraversion-50)/3+(conscientious-50)/4, 0, 95))
	in[InterestTravel] = uint8(clampInt(int(in[InterestTravel])+(openness-50)/2+(extraversion-50)/4, 0, 95))
	in[InterestTechnology] = uint8(clampInt(int(in[InterestTechnology])+(openness-50)/3, 0, 95))
	in[InterestNature] = uint8(clampInt(int(in[InterestNature])+(openness-50)/4+(agreeableness-50)/4, 0, 95))
	in[InterestCraft] = uint8(clampInt(int(in[InterestCraft])+(conscientious-50)/3, 0, 95))
	in[InterestCooking] = uint8(clampInt(int(in[InterestCooking])+(agreeableness-50)/4+(conscientious-50)/4, 0, 95))
	in[InterestGambling] = uint8(clampInt(int(in[InterestGambling])+(extraversion-50)/4+(100-conscientious)/3, 0, 95))
	in[InterestMedicine] = uint8(clampInt(int(in[InterestMedicine])+(agreeableness-50)/3+(conscientious-50)/4, 0, 95))
}

func applyProfessionInterestBias(in *Interests, prof ProfessionType) {
	switch prof {
	case ProfWarrior:
		in[InterestSports] = uint8(clampInt(int(in[InterestSports])+20, 0, 95))
		in[InterestTravel] = uint8(clampInt(int(in[InterestTravel])+15, 0, 95))
	case ProfMage:
		in[InterestReading] = uint8(clampInt(int(in[InterestReading])+25, 0, 95))
		in[InterestTechnology] = uint8(clampInt(int(in[InterestTechnology])+15, 0, 95))
	case ProfRogue:
		in[InterestGambling] = uint8(clampInt(int(in[InterestGambling])+20, 0, 95))
		in[InterestCraft] = uint8(clampInt(int(in[InterestCraft])+10, 0, 95))
	case ProfPriest:
		in[InterestMedicine] = uint8(clampInt(int(in[InterestMedicine])+20, 0, 95))
		in[InterestReading] = uint8(clampInt(int(in[InterestReading])+15, 0, 95))
	case ProfMerchant:
		in[InterestTravel] = uint8(clampInt(int(in[InterestTravel])+20, 0, 95))
		in[InterestSocial] = uint8(clampInt(int(in[InterestSocial])+15, 0, 95))
	case ProfFarmer:
		in[InterestNature] = uint8(clampInt(int(in[InterestNature])+20, 0, 95))
		in[InterestCooking] = uint8(clampInt(int(in[InterestCooking])+10, 0, 95))
	case ProfBlacksmith:
		in[InterestCraft] = uint8(clampInt(int(in[InterestCraft])+25, 0, 95))
		in[InterestSports] = uint8(clampInt(int(in[InterestSports])+10, 0, 95))
	case ProfScribe:
		in[InterestReading] = uint8(clampInt(int(in[InterestReading])+30, 0, 95))
		in[InterestArt] = uint8(clampInt(int(in[InterestArt])+10, 0, 95))
	case ProfNoble:
		in[InterestSocial] = uint8(clampInt(int(in[InterestSocial])+20, 0, 95))
		in[InterestArt] = uint8(clampInt(int(in[InterestArt])+15, 0, 95))
		in[InterestMusic] = uint8(clampInt(int(in[InterestMusic])+15, 0, 95))
	case ProfEntertainer:
		in[InterestMusic] = uint8(clampInt(int(in[InterestMusic])+25, 0, 95))
		in[InterestSocial] = uint8(clampInt(int(in[InterestSocial])+20, 0, 95))
		in[InterestArt] = uint8(clampInt(int(in[InterestArt])+15, 0, 95))
	case ProfRanger:
		in[InterestNature] = uint8(clampInt(int(in[InterestNature])+25, 0, 95))
		in[InterestTravel] = uint8(clampInt(int(in[InterestTravel])+15, 0, 95))
	case ProfAlchemist:
		in[InterestCraft] = uint8(clampInt(int(in[InterestCraft])+20, 0, 95))
		in[InterestReading] = uint8(clampInt(int(in[InterestReading])+15, 0, 95))
	case ProfCook:
		in[InterestCooking] = uint8(clampInt(int(in[InterestCooking])+30, 0, 95))
		in[InterestSocial] = uint8(clampInt(int(in[InterestSocial])+10, 0, 95))
	case ProfHealer:
		in[InterestMedicine] = uint8(clampInt(int(in[InterestMedicine])+25, 0, 95))
		in[InterestNature] = uint8(clampInt(int(in[InterestNature])+10, 0, 95))
	case ProfTailor:
		in[InterestCraft] = uint8(clampInt(int(in[InterestCraft])+20, 0, 95))
		in[InterestArt] = uint8(clampInt(int(in[InterestArt])+15, 0, 95))
	case ProfMiner, ProfWoodcutter:
		in[InterestSports] = uint8(clampInt(int(in[InterestSports])+15, 0, 95))
		in[InterestNature] = uint8(clampInt(int(in[InterestNature])+10, 0, 95))
	}
}

func randNormal(rng *rand.Rand, mean, stddev float64) float64 {
	u1 := rng.Float64()
	u2 := rng.Float64()
	z := mean + stddev*sqrt(-2*log(u1))*cos(2*pi*u2)
	return z
}

const pi = 3.14159265358979323846

func sqrt(x float64) float64 {
	z := x
	for i := 0; i < 10; i++ {
		if z == 0 {
			return 0
		}
		z = (z + x/z) / 2
	}
	return z
}

func log(x float64) float64 {
	if x <= 0 {
		return -1e9
	}
	n := 0.0
	for x >= 3 {
		x /= 2.718281828
		n++
	}
	x--
	f := x
	term := x
	for i := 2; i < 20; i++ {
		term *= -x * float64(i-1) / float64(i)
		f += term
	}
	return n + f
}

func cos(x float64) float64 {
	for x < -pi {
		x += 2 * pi
	}
	for x > pi {
		x -= 2 * pi
	}
	x2 := x * x
	result := 1.0
	term := 1.0
	for i := 1; i < 10; i++ {
		term *= -x2 / float64(2*i*(2*i-1))
		result += term
	}
	return result
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
