package seed

import "myzhihu2/repository/model"

// topicOrder fixes the mid-tier ID blocks: tech 100-105, food 106-111,
// travel 112-117, photo 118-123, fitness 124-129.
var topicOrder = []string{model.TopicTech, model.TopicFood, model.TopicTravel, model.TopicPhoto, model.TopicFitness}

type persona struct {
	username, display, bio string
}

var fixedPersonas = map[uint]persona{
	BigVTechID: {"demo_bigv", "极客老K",
		"科技博主（演示大 V）。演示环境中粉丝数超过 100 即为大 V：我发布的内容不会推送到每个粉丝的收件箱，而是在粉丝刷新时从我的发件箱拉取。"},
	BigVFoodID:   {"bigv_food", "深夜食堂阿明", "美食博主（大 V）。记录家常菜和一人食，内容通过拉模式进入粉丝的首页。"},
	BigVTravelID: {"bigv_travel", "背包客小鹿", "旅行博主（大 V）。演示普通用户没有关注我，所以我的内容只会出现在他的“发现”里。"},
	ReaderID: {"demo_reader", "路过的读者",
		"演示普通用户：只浏览、不发帖。关注了两位大 V 和六位作者，点赞过一些摄影作品，所以首页会出现关注、兴趣、发现三种来源。"},
}

var midDisplayNames = map[string][]string{
	model.TopicTech:    {"代码搬运工", "云原生小林", "数据库老王", "安全研究员阿杰", "后端架构笔记", "AI 应用观察"},
	model.TopicFood:    {"厨房小白成长记", "一人食日记", "家常菜研究所", "烘焙与酸奶", "饺子铺二代", "上班族便当"},
	model.TopicTravel:  {"城市漫步者", "廉航达人", "一个人的旅行", "自驾去远方", "精打细算游", "海岛潜水员"},
	model.TopicPhoto:   {"夜色摄影手记", "街头观察者", "光影食堂", "风光前景控", "慢门爱好者", "黑白胶片"},
	model.TopicFitness: {"久坐救星", "五公里起跑", "力量训练入门", "核心与睡眠", "居家健身", "跑者补给站"},
}

var topicLabel = map[string]string{
	model.TopicTech: "科技", model.TopicFood: "美食", model.TopicTravel: "旅行",
	model.TopicPhoto: "摄影", model.TopicFitness: "健身",
}

// Ordinary display names are combined from these two lists.
var namePrefixes = []string{"早起的", "爱喝茶的", "周末的", "迷路的", "认真的", "慢吞吞的", "好奇的", "安静的", "快乐的", "失眠的", "路边的", "靠窗的"}
var nameSuffixes = []string{"小鱼", "橘子", "阿飞", "木木", "考拉", "星星", "小满", "阿青", "豆包", "柚子", "南风", "小葵", "阿远", "团子"}

// Filler templates: titles are "{subject}{suffix}", bodies are assembled from
// the sentence bank. They exist for volume (author pages, deep pagination,
// load tests) and are kept out of the explore pool by their age.
var fillerSubjects = map[string][]string{
	model.TopicTech:    {"接口超时排查", "日志规范", "单元测试", "代码评审", "服务拆分", "配置管理", "灰度发布", "监控告警", "数据迁移", "缓存设计"},
	model.TopicFood:    {"早餐搭配", "砂锅粥", "凉拌菜", "炖汤", "烤箱菜", "腌菜", "手擀面", "便当", "甜品", "家常小炒"},
	model.TopicTravel:  {"古城游", "露营", "海边小城", "山间徒步", "博物馆之旅", "夜市", "跨年旅行", "说走就走", "亲子游", "公路旅行"},
	model.TopicPhoto:   {"窗边人像", "雨天街拍", "城市夜景", "花卉微距", "胶片质感", "日落剪影", "建筑线条", "宠物抓拍", "旅行记录", "静物小品"},
	model.TopicFitness: {"晨跑", "核心训练", "拉伸放松", "饮食记录", "跳绳", "游泳", "体态调整", "徒步", "骑行", "瑜伽"},
}

var fillerSuffixes = []string{"的一点心得", "踩过的坑", "入门笔记", "新手记录", "这周的尝试", "小结", "的几个细节", "复盘"}

var fillerSentences = map[string][]string{
	model.TopicTech: {
		"这周在项目里遇到了一个小问题，记录一下排查过程。", "先复现问题，再缩小范围，最后才动手修改。",
		"很多时候问题不在代码本身，而在配置和环境。", "加上日志和指标之后，定位速度快了很多。",
		"改完之后补了一个测试用例，防止以后再次出现。", "团队里约定好规范，比事后返工省事得多。",
		"文档写清楚，下一个接手的人会感谢你。", "性能问题一定要先测量，不要凭感觉优化。",
	},
	model.TopicFood: {
		"今天又试了一次，比上回成功一些。", "火候是最难掌握的部分，需要多练几次。",
		"食材新鲜比调料重要。", "提前把配料准备好，下锅时就不会手忙脚乱。",
		"家里人说味道不错，下次可以多做一点。", "少油少盐也可以很好吃。",
		"记下了每一步的时间，方便下次参考。", "剩下的可以冷藏，第二天做成便当。",
	},
	model.TopicTravel: {
		"出发前做了简单的攻略，但最好的部分都是意外发现的。", "早上人少，拍照和散步都很舒服。",
		"当地的小吃比想象中好吃。", "住的地方离车站很近，省了不少时间。",
		"天气变化快，一定要带件外套。", "下次想换个季节再来一次。",
		"跟当地人聊了几句，推荐的地方果然没错。", "这次的预算控制得还不错。",
	},
	model.TopicPhoto: {
		"光线是这张照片最重要的部分。", "换了几个角度，最后还是低机位最好看。",
		"后期只做了简单的调色。", "等了很久才等到合适的时机。",
		"构图上尽量保持简洁。", "手机也能拍出不错的效果。",
		"这组照片想表达一种安静的感觉。", "下次想试试不同的焦段。",
	},
	model.TopicFitness: {
		"坚持了一个月，感觉体力明显变好了。", "动作标准比次数更重要。",
		"训练后记得补充水分和蛋白质。", "今天状态一般，就降低了强度。",
		"睡眠好的时候，训练表现也更好。", "循序渐进，不要急于求成。",
		"记录训练数据能看到自己的进步。", "热身和拉伸都没有偷懒。",
	},
}

var commentBank = []string{
	"写得很清楚，收藏了。", "请问有没有更详细的步骤？", "学到了，谢谢分享！", "我也遇到过类似的情况。",
	"这个思路不错，下次试试。", "补充一点：要注意细节。", "看完马上去实践了。", "期待更多类似的内容。",
}
