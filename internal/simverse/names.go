package simverse

import (
	"math/rand"
)

type NamingCulture uint8

const (
	NameCultureChinese NamingCulture = 0
	NameCultureEnglish NamingCulture = 1
	NameCultureElven    NamingCulture = 2
	NameCultureDwarven  NamingCulture = 3
	NameCultureOrcish   NamingCulture = 4
	NameCultureMax                   = 5
)

type Gender uint8

const (
	GenderMale     Gender = 0
	GenderFemale   Gender = 1
	GenderNonBinary Gender = 2
	GenderMax              = 3
)

type GenderIdentity uint8

const (
	GenderIdentCisMale    GenderIdentity = 0
	GenderIdentCisFemale  GenderIdentity = 1
	GenderIdentTransMale  GenderIdentity = 2
	GenderIdentTransFemale GenderIdentity = 3
	GenderIdentNonBinary   GenderIdentity = 4
	GenderIdentMax                      = 5
)

type SexualOrientation uint8

const (
	SexOrientHetero SexualOrientation = 0
	SexOrientHomo   SexualOrientation = 1
	SexOrientBi     SexualOrientation = 2
	SexOrientAsexual SexualOrientation = 3
	SexOrientMax                   = 4
)

type LifeStage uint8

const (
	LifeStageInfant    LifeStage = 0
	LifeStageChild     LifeStage = 1
	LifeStageAdolescent LifeStage = 2
	LifeStageYoungAdult LifeStage = 3
	LifeStageMiddleAged LifeStage = 4
	LifeStageElderly   LifeStage = 5
	LifeStageCentenarian LifeStage = 6
	LifeStageMax                  = 7
)

var chineseSurnames = [...]string{
	"王", "李", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴",
	"徐", "孙", "胡", "朱", "高", "林", "何", "郭", "马", "罗",
	"梁", "宋", "郑", "谢", "韩", "唐", "冯", "于", "董", "萧",
	"程", "曹", "袁", "邓", "许", "傅", "沈", "曾", "彭", "吕",
	"苏", "卢", "蒋", "蔡", "贾", "丁", "魏", "薛", "叶", "阎",
	"余", "潘", "杜", "戴", "夏", "钟", "汪", "田", "任", "姜",
	"范", "方", "石", "姚", "谭", "廖", "邹", "熊", "金", "陆",
	"郝", "孔", "白", "崔", "康", "毛", "邱", "秦", "江", "史",
	"顾", "侯", "邵", "孟", "龙", "万", "段", "雷", "钱", "汤",
	"尹", "黎", "易", "常", "武", "乔", "贺", "赖", "龚", "文",
}

var chineseGivenNameChars = [...]string{
	"伟", "芳", "娜", "敏", "静", "丽", "强", "磊", "军", "洋",
	"勇", "艳", "杰", "娟", "涛", "明", "超", "秀英", "霞", "平",
	"刚", "桂英", "华", "飞", "玉兰", "桂兰", "玲", "桂珍", "健", "俊",
	"凯", "浩", "宇", "轩", "博", "睿", "晨", "辰", "然", "涵",
	"诗", "琪", "瑶", "欣", "悦", "妍", "茜", "琳", "雪", "晴",
	"皓", "博", "诚", "德", "智", "信", "仁", "义", "礼", "道",
	"文", "武", "山", "河", "海", "天", "星", "月", "日", "云",
	"龙", "凤", "虎", "豹", "鹏", "鹰", "燕", "莺", "蝶", "蝉",
	"金", "银", "玉", "珠", "宝", "珍", "瑶", "琼", "琳", "瑜",
	"梅", "兰", "竹", "菊", "松", "柏", "桃", "柳", "荷", "莲",
	"春", "夏", "秋", "冬", "东", "南", "西", "北", "中", "正",
	"光", "明", "辉", "煌", "耀", "灿", "烂", "晨", "暮", "夜",
	"安", "平", "和", "顺", "康", "健", "福", "禄", "寿", "喜",
	"思", "念", "想", "忆", "怀", "志", "愿", "望", "盼", "梦",
	"清", "雅", "秀", "慧", "聪", "明", "睿", "智", "颖", "悟",
	"坚", "毅", "勇", "敢", "果", "决", "刚", "强", "韧", "恒",
	"谦", "和", "厚", "宽", "容", "仁", "慈", "善", "良", "温",
	"豪", "爽", "洒", "脱", "潇", "洒", "飘", "逸", "凌", "云",
	"紫", "蓝", "青", "绿", "黄", "橙", "红", "粉", "白", "黑",
	"长", "短", "高", "低", "大", "小", "宽", "窄", "深", "浅",
}

var englishSurnames = [...]string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
	"Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas",
	"Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson", "White",
	"Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker", "Young",
	"Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
	"Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
	"Carter", "Roberts", "Gomez", "Phillips", "Evans", "Turner", "Diaz", "Parker",
	"Cruz", "Edwards", "Collins", "Reyes", "Stewart", "Morris", "Morales", "Murphy",
	"Cook", "Rogers", "Gutierrez", "Ortiz", "Morgan", "Cooper", "Peterson", "Bailey",
	"Reed", "Kelly", "Howard", "Ward", "Cox", "Diaz", "Richardson", "Wood",
}

var englishMaleNames = [...]string{
	"James", "John", "Robert", "Michael", "William", "David", "Richard", "Joseph",
	"Thomas", "Charles", "Christopher", "Daniel", "Matthew", "Anthony", "Mark", "Donald",
	"Steven", "Paul", "Andrew", "Joshua", "Kenneth", "Kevin", "Brian", "George",
	"Edward", "Ronald", "Timothy", "Jason", "Jeffrey", "Ryan", "Jacob", "Gary",
	"Nicholas", "Eric", "Jonathan", "Stephen", "Larry", "Justin", "Scott", "Brandon",
}

var englishFemaleNames = [...]string{
	"Mary", "Patricia", "Jennifer", "Linda", "Elizabeth", "Barbara", "Susan", "Jessica",
	"Sarah", "Karen", "Lisa", "Nancy", "Betty", "Margaret", "Sandra", "Ashley",
	"Kimberly", "Emily", "Donna", "Michelle", "Carol", "Amanda", "Dorothy", "Melissa",
	"Deborah", "Stephanie", "Rebecca", "Sharon", "Laura", "Cynthia", "Kathleen", "Amy",
	"Angela", "Shirley", "Anna", "Brenda", "Pamela", "Emma", "Nicole", "Helen",
}

func GenerateChineseName(rng *rand.Rand, gender Gender) string {
	surname := chineseSurnames[rng.Intn(len(chineseSurnames))]
	doubleName := rng.Intn(100) < 70

	if doubleName {
		c1 := chineseGivenNameChars[rng.Intn(len(chineseGivenNameChars))]
		c2 := chineseGivenNameChars[rng.Intn(len(chineseGivenNameChars))]
		return surname + c1 + c2
	} else {
		c := chineseGivenNameChars[rng.Intn(len(chineseGivenNameChars))]
		return surname + c
	}
}

func GenerateEnglishName(rng *rand.Rand, gender Gender) (string, string) {
	surname := englishSurnames[rng.Intn(len(englishSurnames))]
	var givenName string
	switch gender {
	case GenderMale:
		givenName = englishMaleNames[rng.Intn(len(englishMaleNames))]
	case GenderFemale:
		givenName = englishFemaleNames[rng.Intn(len(englishFemaleNames))]
	default:
		if rng.Intn(2) == 0 {
			givenName = englishMaleNames[rng.Intn(len(englishMaleNames))]
		} else {
			givenName = englishFemaleNames[rng.Intn(len(englishFemaleNames))]
		}
	}
	return givenName, surname
}

func ChineseNameCombinations() uint64 {
	surnames := uint64(len(chineseSurnames))
	chars := uint64(len(chineseGivenNameChars))
	single := surnames * chars
	double := surnames * chars * chars
	return single + double
}

func EnglishNameCombinations() uint64 {
	surnames := uint64(len(englishSurnames))
	male := uint64(len(englishMaleNames))
	female := uint64(len(englishFemaleNames))
	return surnames * (male + female)
}

func CollisionProbability(n uint64, total uint64) float64 {
	if n > total {
		return 1.0
	}
	prob := 1.0
	for i := uint64(0); i < n; i++ {
		prob *= float64(total-i) / float64(total)
	}
	return 1.0 - prob
}

func GetLifeStage(age uint16, species SpeciesType) LifeStage {
	baseAges := [SpeciesMax][LifeStageMax]uint16{
		SpeciesHuman:     {3, 12, 18, 35, 55, 75, 100},
		SpeciesElf:       {10, 50, 100, 200, 400, 600, 1000},
		SpeciesDwarf:     {5, 20, 40, 80, 150, 250, 400},
		SpeciesOrc:       {2, 8, 14, 25, 40, 55, 80},
		SpeciesBeastman:  {2, 10, 16, 28, 45, 65, 90},
		SpeciesDragonkin: {20, 80, 150, 300, 600, 1000, 2000},
		SpeciesFey:       {5, 25, 50, 100, 200, 300, 500},
		SpeciesUndead:    {0, 0, 0, 0, 0, 0, 0},
	}

	thresholds := baseAges[species]
	for stage := LifeStage(0); stage < LifeStageCentenarian; stage++ {
		if age < thresholds[stage] {
			return stage
		}
	}
	return LifeStageCentenarian
}

func StageUpdateInterval(stage LifeStage) uint32 {
	switch stage {
	case LifeStageInfant:
		return 100
	case LifeStageChild:
		return 500
	case LifeStageAdolescent:
		return 1000
	case LifeStageYoungAdult:
		return 2000
	case LifeStageMiddleAged:
		return 3000
	case LifeStageElderly:
		return 2000
	case LifeStageCentenarian:
		return 5000
	default:
		return 10000
	}
}

func LifeEventCountPerStage(stage LifeStage) float64 {
	switch stage {
	case LifeStageInfant:
		return 0.1
	case LifeStageChild:
		return 0.5
	case LifeStageAdolescent:
		return 1.0
	case LifeStageYoungAdult:
		return 3.0
	case LifeStageMiddleAged:
		return 2.0
	case LifeStageElderly:
		return 1.0
	case LifeStageCentenarian:
		return 0.3
	default:
		return 0.5
	}
}
