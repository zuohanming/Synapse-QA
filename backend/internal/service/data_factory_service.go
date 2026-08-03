package service

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

type DataFactoryService struct {
	rng *rand.Rand
}

func NewDataFactoryService() *DataFactoryService {
	return &DataFactoryService{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// ------------------------------- 姓氏库 -------------------------------

var surnames = []string{
	"赵", "钱", "孙", "李", "周", "吴", "郑", "王", "冯", "陈", "褚", "卫", "蒋", "沈", "韩", "杨",
	"朱", "秦", "尤", "许", "何", "吕", "施", "张", "孔", "曹", "严", "华", "金", "魏", "陶", "姜",
	"戚", "谢", "邹", "喻", "柏", "水", "窦", "章", "云", "苏", "潘", "葛", "奚", "范", "彭", "郎",
	"鲁", "韦", "昌", "马", "苗", "凤", "花", "方", "俞", "任", "袁", "柳", "丰", "鲍", "史", "唐",
	"费", "廉", "岑", "薛", "雷", "贺", "倪", "汤", "滕", "殷", "罗", "毕", "郝", "安", "常", "乐",
	"于", "时", "傅", "皮", "卞", "齐", "康", "伍", "余", "元", "顾", "孟", "平", "黄", "和", "穆",
	"萧", "尹", "姚", "邵", "湛", "汪", "祁", "毛", "禹", "狄", "米", "贝", "明", "臧", "计", "伏",
	"成", "戴", "谈", "宋", "茅", "庞", "熊", "纪", "舒", "屈", "项", "祝", "董", "梁", "杜", "阮",
	"蓝", "闵", "席", "季", "麻", "强", "贾", "路", "娄", "危", "江", "童", "颜", "郭", "梅", "盛",
	"林", "刁", "钟", "徐", "邱", "骆", "高", "夏", "蔡", "田", "樊", "胡", "凌", "霍", "虞", "万",
	"支", "柯", "咎", "管", "卢", "莫", "经", "房", "裘", "缪", "干", "解", "应", "宗", "丁", "宣",
	"贲", "邓", "郁", "单", "杭", "洪", "包", "诸", "左", "石", "崔", "吉", "钮", "龚", "程", "嵇",
	"邢", "滑", "裴", "陆", "荣", "翁", "荀", "羊", "于", "惠", "甄", "曲", "家", "封", "芮", "羿",
	"储", "靳", "汲", "邴", "糜", "松", "井", "段", "富", "巫", "乌", "焦", "巴", "弓", "牧", "隗",
	"山", "谷", "车", "侯", "宓", "蓬", "全", "郗", "班", "仰", "秋", "仲", "伊", "宫", "宁", "仇",
	"栾", "暴", "甘", "钭", "厉", "戎", "祖", "武", "符", "刘", "景", "詹", "束", "龙", "叶", "幸",
	"司", "韶", "郜", "黎", "蓟", "薄", "印", "宿", "白", "怀", "蒲", "台", "从", "鄂", "索", "咸",
	"籍", "赖", "卓", "蔺", "屠", "蒙", "池", "乔", "阴", "胥", "能", "苍", "双", "闻", "莘", "党",
	"翟", "谭", "贡", "劳", "逄", "姬", "申", "扶", "堵", "冉", "宰", "郦", "雍", "璩", "桑", "桂",
	"濮", "牛", "寿", "通", "边", "扈", "燕", "冀", "浦", "尚", "农", "温", "别", "庄", "晏", "柴",
	"瞿", "阎", "充", "慕", "连", "茹", "习", "宦", "艾", "鱼", "容", "向", "古", "易", "慎", "戈",
	"廖", "庚", "终", "暨", "居", "衡", "步", "都", "耿", "满", "弘", "匡", "国", "文", "寇", "广",
	"禄", "阙", "东", "欧", "殳", "沃", "利", "蔚", "越", "夔", "隆", "师", "巩", "厍", "聂", "晁",
	"勾", "敖", "融", "冷", "訾", "辛", "阚", "那", "简", "饶", "空", "曾", "母", "沙", "乜", "养",
	"鞠", "须", "丰", "巢", "关", "蒯", "相", "查", "后", "荆", "红", "游", "竺", "权", "逯", "盖",
	"益", "桓", "公", "万俟", "司马", "上官", "欧阳", "夏侯", "诸葛", "闻人", "东方", "赫连", "皇甫",
	"尉迟", "公羊", "澹台", "公冶", "宗政", "濮阳", "淳于", "单于", "太叔", "申屠", "公孙", "仲孙",
	"轩辕", "令狐", "钟离", "宇文", "长孙", "慕容", "鲜于", "闾丘", "司徒", "司空", "丌官", "司寇",
	"仉", "督", "子车", "颛孙", "端木", "巫马", "公西", "漆雕", "乐正", "壤驷", "公良", "拓拔",
	"夹谷", "宰父", "谷梁", "晋", "楚", "闫", "法", "汝", "鄢", "涂", "钦", "段干", "百里", "东郭",
	"南门", "呼延", "归", "海", "羊舌", "微生", "岳", "帅", "缑", "亢", "况", "后", "有", "琴",
	"梁丘", "左丘", "东门", "西门", "商", "牟", "佘", "佴", "伯", "赏", "南宫", "墨", "哈", "谯",
	"笪", "年", "爱", "阳", "佟", "第五", "言", "福",
}

var maleGiven = []string{
	"伟", "强", "磊", "军", "勇", "杰", "涛", "明", "辉", "鹏",
	"浩", "峰", "文", "刚", "斌", "波", "飞", "超", "亮", "龙",
	"林", "华", "敏", "鑫", "宇", "宁", "建", "志", "成", "毅",
	"俊", "健", "旭", "睿", "翔", "凯", "泽", "平", "安", "恒",
	"铭", "睿", "晨", "锐", "翰", "子", "哲", "博", "慕", "景",
	"嘉", "宸", "轩", "逸", "然", "言", "谦", "裕", "晖", "启",
	"靖", "誉", "镇", "邦", "彦", "康", "德", "誉", "啸", "知",
	"廷", "晏", "枫", "锦", "仕", "奕", "峻", "梧", "延", "霆",
}

var femaleGiven = []string{
	"芳", "敏", "静", "丽", "婷", "雪", "玲", "萍", "红", "霞",
	"娜", "慧", "琴", "梅", "洁", "云", "燕", "珍", "莉", "丹",
	"秀", "瑛", "文", "瑶", "艺", "涵", "冰", "洋", "倩", "琳",
	"蓉", "悦", "雨", "曼", "淑", "婉", "仪", "妍", "怡", "媛",
	"诗", "雅", "晶", "萌", "笑", "清", "晔", "菱", "蕾", "欣",
	"芊", "芷", "姝", "若", "灵", "念", "芮", "茹", "琪", "琼",
	"佳", "语", "芸", "心", "荷", "嫒", "晴", "欢", "娇", "棠",
	"素", "宜", "滢", "涟", "梦", "嘉", "露", "湘", "漪", "宁",
}

var phonePrefixes = []string{"130", "131", "132", "133", "134", "135", "136", "137", "138", "139",
	"150", "151", "152", "153", "155", "156", "157", "158", "159",
	"170", "176", "177", "178",
	"180", "181", "182", "183", "184", "185", "186", "187", "188", "189",
	"191", "193", "195", "196", "197", "198", "199"}

var emailDomains = []string{"test.com", "example.com", "mock.com", "qa.com", "synapse.com", "demo.com"}

var cities = []string{"北京市", "上海市", "广州市", "深圳市", "杭州市", "成都市", "武汉市", "南京市", "重庆市", "西安市"}

var districts = []string{"朝阳区", "浦东新区", "天河区", "南山区", "西湖区", "武侯区", "洪山区", "鼓楼区", "渝中区", "雁塔区"}

var streets = []string{"中山路", "人民路", "解放路", "建设路", "和平路", "长安街", "南京路", "科技路", "创业大道", "滨江路"}

var companies = []string{
	"星辰科技", "云端数据", "智联网络", "极光软件", "磐石信息",
	"浪潮数字", "飞翼通信", "蓝海云", "矩阵互联", "万象智能",
	"鼎新科技", "华信数据", "天工智能", "博远信息", "瑞丰科技",
	"汇智网络", "青云数据", "恒通科技", "神州数码", "启明星辰",
}

var nickNames = []string{
	"清风徐来", "代码诗人", "追梦人", "夜空中最亮的星", "一米阳光",
	"岁月静好", "浮生若梦", "向阳而生", "星辰大海", "简单快乐",
	"不忘初心", "行者无疆", "海阔天空", "小小书虫", "天空之城",
	"梦想家", "小鱼儿", "流年似水", "时光旅人", "奔跑的蜗牛",
}

// ------------------------------- 生成器注册表 -------------------------------

func (s *DataFactoryService) ListGenerators() []model.MockGenerator {
	return []model.MockGenerator{
		// 基础
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{name}}", Name: "中文姓名", Description: "随机生成 2-4 字中文姓名", Syntax: "{{name}}", Example: s.genName()},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{phone}}", Name: "手机号", Description: "随机 11 位手机号（13/15/17/18/19 开头）", Syntax: "{{phone}}", Example: s.genPhone()},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{email}}", Name: "邮箱", Description: "随机邮箱地址（8位前缀 + 常见域名）", Syntax: "{{email}}", Example: s.genEmail()},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{id_card}}", Name: "身份证号", Description: "符合校验规则的 18 位身份证号", Syntax: "{{id_card}}", Example: s.genIDCard()},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{uuid}}", Name: "UUID", Description: "UUID v4 格式", Syntax: "{{uuid}}", Example: s.genUUID()},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{int(1,100)}}", Name: "随机整数", Description: "指定范围内的随机整数", Syntax: "{{int(min,max)}}", Example: fmt.Sprintf("%d", s.rng.Intn(100)+1)},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{float(0,100,2)}}", Name: "随机浮点数", Description: "指定范围内的浮点数（可控制小数位数）", Syntax: "{{float(min,max,decimals)}}", Example: fmt.Sprintf("%.2f", s.rng.Float64()*100)},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{timestamp}}", Name: "时间戳", Description: "当前毫秒级 Unix 时间戳", Syntax: "{{timestamp}} 或 {{timestamp(\"offset\")}}", Example: fmt.Sprintf("%d", time.Now().UnixMilli())},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{date(\"Y-m-d\")}}", Name: "日期时间", Description: "按格式生成日期时间字符串", Syntax: "{{date(\"format\")}}", Example: time.Now().Format("2006-01-02")},
		{Category: "basic", CategoryCN: "基础", Placeholder: "{{lorem(30)}}", Name: "随机文本", Description: "生成指定长度的中文文本", Syntax: "{{lorem(length)}}", Example: s.genLorem(30)},

		// 业务
		{Category: "business", CategoryCN: "业务", Placeholder: "{{seq(\"ORD\",1,1)}}", Name: "递增序号", Description: "按步长递增的序号（可加前缀）", Syntax: "{{seq(\"prefix\",start,step)}}", Example: "ORD1"},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{enum(\"A\",\"B\",\"C\")}}", Name: "枚举选取", Description: "从枚举列表中随机选取一个值", Syntax: "{{enum(\"v1\",\"v2\",...)}}", Example: "A"},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{nickname}}", Name: "昵称", Description: "随机中文昵称", Syntax: "{{nickname}}", Example: s.genNickname()},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{address}}", Name: "地址", Description: "随机完整地址", Syntax: "{{address}}", Example: s.genAddress()},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{company}}", Name: "公司名", Description: "随机公司名称", Syntax: "{{company}}", Example: s.genCompany()},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{bank_card}}", Name: "银行卡号", Description: "通过 Luhn 校验的银行卡号", Syntax: "{{bank_card}}", Example: s.genBankCard()},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{url}}", Name: "URL", Description: "随机 URL 地址", Syntax: "{{url}}", Example: s.genURL()},
		{Category: "business", CategoryCN: "业务", Placeholder: "{{ip}}", Name: "IP 地址", Description: "随机 IPv4 地址", Syntax: "{{ip}}", Example: s.genIP()},

		// 进阶
		{Category: "advanced", CategoryCN: "进阶", Placeholder: "{{img(200,300)}}", Name: "图片URL", Description: "随机占位图片 URL", Syntax: "{{img(width,height)}}", Example: fmt.Sprintf("https://picsum.photos/%d/%d", 200, 300)},
		{Category: "advanced", CategoryCN: "进阶", Placeholder: "{{color}}", Name: "颜色值", Description: "随机 Hex 颜色值", Syntax: "{{color}}", Example: s.genColor()},
		{Category: "advanced", CategoryCN: "进阶", Placeholder: "{{bool}}", Name: "布尔值", Description: "随机 true 或 false", Syntax: "{{bool}}", Example: "true"},
		{Category: "advanced", CategoryCN: "进阶", Placeholder: "{{int_seq(\"key\",0,1)}}", Name: "跨接口序列", Description: "同一批次内共享的递增计数器", Syntax: "{{int_seq(\"counter_key\",start,step)}}", Example: "0"},

		// 场景
		{Category: "scene", CategoryCN: "场景", Placeholder: "{{person}}", Name: "个人信息", Description: "生成完整个人信息 JSON 对象", Syntax: "{{person}}", Example: fmt.Sprintf(`{"name":"%s","phone":"%s","email":"%s"}`, s.genName(), s.genPhone(), s.genEmail())},
	}
}

// ------------------------------- 预览 -------------------------------

func (s *DataFactoryService) Preview(placeholder string, count int) *model.MockPreviewResponse {
	if count <= 0 {
		count = 5
	}
	if count > 100 {
		count = 100
	}

	results := make([]string, count)
	for i := 0; i < count; i++ {
		results[i] = s.generate(placeholder)
	}
	return &model.MockPreviewResponse{Placeholder: placeholder, Results: results}
}

// ------------------------------- 单值生成 -------------------------------

func (s *DataFactoryService) generate(placeholder string) string {
	switch {
	case placeholder == "{{name}}":
		return s.genName()
	case placeholder == "{{phone}}":
		return s.genPhone()
	case placeholder == "{{email}}":
		return s.genEmail()
	case placeholder == "{{id_card}}":
		return s.genIDCard()
	case placeholder == "{{uuid}}":
		return s.genUUID()
	case placeholder == "{{nickname}}":
		return s.genNickname()
	case placeholder == "{{address}}":
		return s.genAddress()
	case placeholder == "{{company}}":
		return s.genCompany()
	case placeholder == "{{bank_card}}":
		return s.genBankCard()
	case placeholder == "{{url}}":
		return s.genURL()
	case placeholder == "{{ip}}":
		return s.genIP()
	case placeholder == "{{color}}":
		return s.genColor()
	case placeholder == "{{bool}}":
		if s.rng.Intn(2) == 0 {
			return "true"
		}
		return "false"
	case placeholder == "{{timestamp}}":
		return fmt.Sprintf("%d", time.Now().UnixMilli())
	case strings.HasPrefix(placeholder, "{{timestamp("):
		return s.genTimestampOffset(placeholder)
	case strings.HasPrefix(placeholder, "{{date("):
		return s.genDate(placeholder)
	case strings.HasPrefix(placeholder, "{{int("):
		return s.genInt(placeholder)
	case strings.HasPrefix(placeholder, "{{float("):
		return s.genFloat(placeholder)
	case strings.HasPrefix(placeholder, "{{int_seq("):
		return s.genIntSeq(placeholder)
	case strings.HasPrefix(placeholder, "{{seq("):
		return s.genSeq(placeholder)
	case strings.HasPrefix(placeholder, "{{enum("):
		return s.genEnum(placeholder)
	case strings.HasPrefix(placeholder, "{{lorem("):
		return s.genLoremFromPlaceholder(placeholder)
	case strings.HasPrefix(placeholder, "{{img("):
		return s.genImg(placeholder)
	case placeholder == "{{person}}":
		return s.genPerson()
	default:
		return placeholder
	}
}

func (s *DataFactoryService) genName() string {
	surname := surnames[s.rng.Intn(len(surnames))]
	pool := maleGiven
	if s.rng.Intn(2) == 0 {
		pool = femaleGiven
	}
	givenLen := 1 + s.rng.Intn(2) // 1 or 2 chars
	given := ""
	for i := 0; i < givenLen; i++ {
		given += pool[s.rng.Intn(len(pool))]
	}
	return surname + given
}

func (s *DataFactoryService) genPhone() string {
	prefix := phonePrefixes[s.rng.Intn(len(phonePrefixes))]
	suffix := fmt.Sprintf("%08d", s.rng.Intn(100000000))
	return prefix + suffix
}

func (s *DataFactoryService) genEmail() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[s.rng.Intn(len(chars))]
	}
	domain := emailDomains[s.rng.Intn(len(emailDomains))]
	return string(b) + "@" + domain
}

func (s *DataFactoryService) genIDCard() string {
	area := fmt.Sprintf("%06d", s.rng.Intn(999999))
	birth := fmt.Sprintf("19%02d%02d%02d", s.rng.Intn(99), 1+s.rng.Intn(12), 1+s.rng.Intn(28))
	seq := fmt.Sprintf("%03d", s.rng.Intn(999))
	base := area + birth + seq
	return base + s.luhnCheckDigit(base)
}

func (s *DataFactoryService) luhnCheckDigit(input string) string {
	sum := 0
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checkMap := []string{"1", "0", "X", "9", "8", "7", "6", "5", "4", "3", "2"}
	for i, ch := range input {
		sum += int(ch-'0') * weights[i]
	}
	return checkMap[sum%11]
}

func (s *DataFactoryService) genUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(s.rng.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *DataFactoryService) genNickname() string {
	return nickNames[s.rng.Intn(len(nickNames))]
}

func (s *DataFactoryService) genAddress() string {
	city := cities[s.rng.Intn(len(cities))]
	district := districts[s.rng.Intn(len(districts))]
	street := streets[s.rng.Intn(len(streets))]
	num := fmt.Sprintf("%d", 1+s.rng.Intn(500))
	return city + district + street + num + "号"
}

func (s *DataFactoryService) genCompany() string {
	suffixes := []string{"有限公司", "科技有限公司", "信息技术有限公司", "网络有限公司", "集团有限公司"}
	company := companies[s.rng.Intn(len(companies))]
	return company + suffixes[s.rng.Intn(len(suffixes))]
}

func (s *DataFactoryService) genBankCard() string {
	bin := []string{"622202", "622848", "622700", "622262", "621700", "621098"}
	prefix := bin[s.rng.Intn(len(bin))]
	body := ""
	for i := 0; i < 9; i++ {
		body += fmt.Sprintf("%01d", s.rng.Intn(10))
	}
	base := prefix + body
	return base + s.luhnCardDigit(base)
}

func (s *DataFactoryService) luhnCardDigit(input string) string {
	sum := 0
	parity := len(input) % 2
	for i, ch := range input {
		d := int(ch - '0')
		if i%2 == parity {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return fmt.Sprintf("%d", (10-sum%10)%10)
}

func (s *DataFactoryService) genURL() string {
	paths := []string{"api", "v1", "page", "user", "order", "product", "detail", "list"}
	path := paths[s.rng.Intn(len(paths))]
	id := fmt.Sprintf("%04x", s.rng.Intn(65536))
	return fmt.Sprintf("https://www.example.com/%s/%s", path, id)
}

func (s *DataFactoryService) genIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", s.rng.Intn(223)+1, s.rng.Intn(256), s.rng.Intn(256), s.rng.Intn(254)+1)
}

func (s *DataFactoryService) genColor() string {
	return fmt.Sprintf("#%02x%02x%02x", s.rng.Intn(256), s.rng.Intn(256), s.rng.Intn(256))
}

var seqCounters = make(map[string]int)

func (s *DataFactoryService) genIntSeq(placeholder string) string {
	re := regexp.MustCompile(`{{int_seq\("([^"]+)",(\d+),(\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return "0"
	}
	key := matches[1]
	start, _ := strconv.Atoi(matches[2])
	step, _ := strconv.Atoi(matches[3])
	current, ok := seqCounters[key]
	if !ok {
		current = start
	}
	seqCounters[key] = current + step
	return fmt.Sprintf("%d", current)
}

func (s *DataFactoryService) genSeq(placeholder string) string {
	re := regexp.MustCompile(`{{seq\("([^"]*)",(\d+),(\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return "1"
	}
	prefix := matches[1]
	start, _ := strconv.Atoi(matches[2])
	step, _ := strconv.Atoi(matches[3])
	current, ok := seqCounters["seq_"+prefix]
	if !ok {
		current = start
	}
	seqCounters["seq_"+prefix] = current + step
	return fmt.Sprintf("%s%d", prefix, current)
}

func (s *DataFactoryService) genEnum(placeholder string) string {
	re := regexp.MustCompile(`{{enum\(([^)]+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return ""
	}
	raw := matches[1]
	// Split by commas not inside quotes
	parts := splitEnum(raw)
	if len(parts) == 0 {
		return ""
	}
	return strings.Trim(parts[s.rng.Intn(len(parts))], "\"")
}

func splitEnum(s string) []string {
	var parts []string
	current := ""
	inQuote := false
	for _, ch := range s {
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if ch == ',' && !inQuote {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	parts = append(parts, current)
	return parts
}

func (s *DataFactoryService) genInt(placeholder string) string {
	re := regexp.MustCompile(`{{int\((\d+),(\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return "0"
	}
	min, _ := strconv.Atoi(matches[1])
	max, _ := strconv.Atoi(matches[2])
	if min >= max {
		return fmt.Sprintf("%d", min)
	}
	return fmt.Sprintf("%d", min+s.rng.Intn(max-min+1))
}

func (s *DataFactoryService) genFloat(placeholder string) string {
	re := regexp.MustCompile(`{{float\((\d+),(\d+),(\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return "0.00"
	}
	min, _ := strconv.ParseFloat(matches[1], 64)
	max, _ := strconv.ParseFloat(matches[2], 64)
	dec, _ := strconv.Atoi(matches[3])
	val := min + s.rng.Float64()*(max-min)
	return fmt.Sprintf("%.*f", dec, val)
}

func (s *DataFactoryService) genTimestampOffset(placeholder string) string {
	re := regexp.MustCompile(`{{timestamp\("([^"]+)"\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	offsetStr := matches[1]
	d, err := parseDuration(offsetStr)
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixMilli())
	}
	return fmt.Sprintf("%d", time.Now().Add(d).UnixMilli())
}

func parseDuration(s string) (time.Duration, error) {
	// Support: -1d, 2h, -30m, etc.
	re := regexp.MustCompile(`(-?\d+)(d|h|m|s)`)
	match := re.FindStringSubmatch(s)
	if match == nil {
		return 0, fmt.Errorf("invalid offset: %s", s)
	}
	val, _ := strconv.Atoi(match[1])
	unit := match[2]
	switch unit {
	case "d":
		return time.Duration(val) * 24 * time.Hour, nil
	case "h":
		return time.Duration(val) * time.Hour, nil
	case "m":
		return time.Duration(val) * time.Minute, nil
	case "s":
		return time.Duration(val) * time.Second, nil
	}
	return 0, fmt.Errorf("unknown unit: %s", unit)
}

func (s *DataFactoryService) genDate(placeholder string) string {
	re := regexp.MustCompile(`{{date\("([^"]+)"\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return time.Now().Format("2006-01-02")
	}
	format := matches[1]
	// Convert PHP-style format to Go format
	format = strings.ReplaceAll(format, "Y", "2006")
	format = strings.ReplaceAll(format, "m", "01")
	format = strings.ReplaceAll(format, "d", "02")
	format = strings.ReplaceAll(format, "H", "15")
	format = strings.ReplaceAll(format, "i", "04")
	format = strings.ReplaceAll(format, "s", "05")
	return time.Now().Format(format)
}

var loremChars = []rune("的一是在不了有和人这中大为上个国我以要他时来用们生到作地于出就分对成会可主发年动同工也能下过子说产种面而方后多定行学法所民得经十三之进着等部度家电力里如水化高自二理起小物现实加量都两体制机当使点从业本去把性好应开它合还因由其些然前外天政四日那社义事平形相全表间样与关各重新线内数正心反你明看原又么利比或但质气第向道命此变条只没结解问意建月公无系军很情者最立代想已通并提直题党程展五果料象员革位入常文总次品式活设及管特件长求老头基资边流路级少图山统接知较将组见计别她手角期根论运农指几九区强放决西被干做必战先回则任取据处队南给色光门即保治北造百规热领七海口东导器压志世金增争济阶油思术极交受联什认六共权收证改清己美再采转更单风切打白教速花带安场身车例真务具万每目至达走积示议声报斗完类八离华名确才科张信马节话米整空元况今集温传土许步群广石记需段研界拉林律叫且究观越织装影算低持音众书布复容儿须际商非验连断深难近矿千周委素技备半办青省列习响约支般史感劳便团往酸历市克何除消构府称太准精值号率族维划选标写存候毛亲快效斯院查江型眼王按格养易置派层片始却专状育厂京识适属圆包火住调满县局照参红细引听该铁价严")

func (s *DataFactoryService) genLorem(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = loremChars[s.rng.Intn(len(loremChars))]
	}
	return string(b)
}

func (s *DataFactoryService) genLoremFromPlaceholder(placeholder string) string {
	re := regexp.MustCompile(`{{lorem\((\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return s.genLorem(30)
	}
	length, _ := strconv.Atoi(matches[1])
	if length <= 0 {
		length = 30
	}
	if length > 5000 {
		length = 5000
	}
	return s.genLorem(length)
}

func (s *DataFactoryService) genImg(placeholder string) string {
	re := regexp.MustCompile(`{{img\((\d+),(\d+)\)}}`)
	matches := re.FindStringSubmatch(placeholder)
	if matches == nil {
		return fmt.Sprintf("https://picsum.photos/%d/%d", 200, 300)
	}
	w := matches[1]
	h := matches[2]
	return fmt.Sprintf("https://picsum.photos/%s/%s?random=%d", w, h, s.rng.Intn(10000))
}

func (s *DataFactoryService) genPerson() string {
	return fmt.Sprintf(`{"name":"%s","phone":"%s","email":"%s","idCard":"%s","address":"%s","company":"%s"}`,
		s.genName(), s.genPhone(), s.genEmail(), s.genIDCard(), s.genAddress(), s.genCompany())
}

// ResetCounters resets all sequence counters (for batch runs)
func (s *DataFactoryService) ResetCounters() {
	seqCounters = make(map[string]int)
}
