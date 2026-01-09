// internal/app/bootstrap/bootstrap.go
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/anzhiyu-c/anheyu-app/ent"
	"github.com/anzhiyu-c/anheyu-app/ent/file"
	"github.com/anzhiyu-c/anheyu-app/ent/link"
	"github.com/anzhiyu-c/anheyu-app/ent/linkcategory"
	"github.com/anzhiyu-c/anheyu-app/ent/setting"
	"github.com/anzhiyu-c/anheyu-app/ent/usergroup"
	"github.com/anzhiyu-c/anheyu-app/internal/configdef"
	"github.com/anzhiyu-c/anheyu-app/internal/pkg/utils"
	"github.com/anzhiyu-c/anheyu-app/pkg/constant"
	"github.com/anzhiyu-c/anheyu-app/pkg/domain/model"
)

type Bootstrapper struct {
	entClient *ent.Client
}

func NewBootstrapper(entClient *ent.Client) *Bootstrapper {
	return &Bootstrapper{
		entClient: entClient,
	}
}

func (b *Bootstrapper) InitializeDatabase() error {
	log.Println("--- 开始执行数据库初始化引导程序 (配置注册表模式) ---")

	if err := b.entClient.Schema.Create(context.Background()); err != nil {
		return fmt.Errorf("数据库 schema 创建/更新失败: %w", err)
	}
	log.Println("--- 数据库 Schema 同步成功 ---")

	b.syncSettings()
	b.initUserGroups()
	b.initStoragePolicies()
	b.initLinks()
	b.initDefaultPages()
	b.checkUserTable()

	log.Println("--- 数据库初始化引导程序执行完成 ---")
	return nil
}

// syncSettings 检查并同步配置项，确保所有在代码中定义的配置项都存在于数据库中。
func (b *Bootstrapper) syncSettings() {
	log.Println("--- 开始同步站点配置 (Setting 表)... ---")
	ctx := context.Background()
	newlyAdded := 0

	// 从 configdef 循环所有定义
	for _, def := range configdef.AllSettings {
		exists, err := b.entClient.Setting.Query().Where(setting.ConfigKey(def.Key.String())).Exist(ctx)
		if err != nil {
			log.Printf("⚠️ 失败: 查询配置项 '%s' 失败: %v", def.Key, err)
			continue
		}

		// 如果配置项在数据库中不存在，则创建它
		if !exists {
			value := def.Value
			// 特殊处理需要动态生成的密钥
			if def.Key == constant.KeyJWTSecret {
				value, _ = utils.GenerateRandomString(32)
			}
			if def.Key == constant.KeyLocalFileSigningSecret {
				value, _ = utils.GenerateRandomString(32)
			}

			// 检查环境变量覆盖
			envKey := "AN_SETTING_DEFAULT_" + strings.ToUpper(string(def.Key))
			if envValue, ok := os.LookupEnv(envKey); ok {
				value = envValue
				log.Printf("    - 配置项 '%s' 由环境变量覆盖。", def.Key)
			}

			_, createErr := b.entClient.Setting.Create().
				SetConfigKey(def.Key.String()).
				SetValue(value).
				SetComment(def.Comment).
				Save(ctx)

			if createErr != nil {
				log.Printf("⚠️ 失败: 新增默认配置项 '%s' 失败: %v", def.Key, createErr)
			} else {
				log.Printf("    -新增配置项: '%s' 已写入数据库。", def.Key)
				newlyAdded++
			}
		}
	}

	if newlyAdded > 0 {
		log.Printf("--- 站点配置同步完成，共新增 %d 个配置项。---", newlyAdded)
	} else {
		log.Println("--- 站点配置同步完成，无需新增配置项。---")
	}
}

// initUserGroups 检查并初始化默认用户组。
func (b *Bootstrapper) initUserGroups() {
	log.Println("--- 开始初始化默认用户组 (UserGroup 表) ---")
	ctx := context.Background()
	for _, groupData := range configdef.AllUserGroups {
		exists, err := b.entClient.UserGroup.Query().Where(usergroup.ID(groupData.ID)).Exist(ctx)
		if err != nil {
			log.Printf("⚠️ 失败: 查询用户组 ID: %d 失败: %v", groupData.ID, err)
			continue
		}
		if !exists {
			_, createErr := b.entClient.UserGroup.Create().
				SetID(groupData.ID).
				SetName(groupData.Name).
				SetDescription(groupData.Description).
				SetPermissions(groupData.Permissions).
				SetMaxStorage(groupData.MaxStorage).
				SetSpeedLimit(groupData.SpeedLimit).
				SetSettings(&groupData.Settings).
				Save(ctx)
			if createErr != nil {
				log.Printf("⚠️ 失败: 创建默认用户组 '%s' (ID: %d) 失败: %v", groupData.Name, groupData.ID, createErr)
			}
		}
	}
	log.Println("--- 默认用户组 (UserGroup 表) 初始化完成。---")
}

func (b *Bootstrapper) initStoragePolicies() {
	log.Println("--- 开始初始化默认存储策略 (StoragePolicy 表) ---")
	ctx := context.Background()
	count, err := b.entClient.StoragePolicy.Query().Count(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 查询存储策略数量失败: %v", err)
		return
	}

	if count == 0 {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("❌ 致命错误: 无法获取当前工作目录: %v", err)
		}
		dirNameRule := filepath.Join(wd, "data/storage")

		settings := model.StoragePolicySettings{
			"chunk_size":    26214400,
			"pre_allocate":  true,
			"upload_method": constant.UploadMethodClient,
		}

		_, err = b.entClient.StoragePolicy.Create().
			SetName("本机存储").
			SetType(string(constant.PolicyTypeLocal)).
			SetBasePath(dirNameRule).
			SetVirtualPath("/").
			SetSettings(settings).
			Save(ctx)

		if err != nil {
			log.Printf("⚠️ 失败: 创建默认存储策略 '本机存储' 失败: %v", err)
		} else {
			log.Printf("✅ 成功: 默认存储策略 '本机存储' 已创建。路径规则: %s", dirNameRule)
		}
	}

	// 确保用户头像存储策略存在（适配系统升级的情况）
	b.ensureAvatarStoragePolicy()

	log.Println("--- 默认存储策略 (StoragePolicy 表) 初始化完成。---")
}

// ensureAvatarStoragePolicy 确保用户头像存储策略存在
// 在系统启动时检查，如果不存在则创建（适配系统升级的情况）
func (b *Bootstrapper) ensureAvatarStoragePolicy() {
	ctx := context.Background()

	// 检查是否已存在用户头像存储策略
	count, err := b.entClient.StoragePolicy.Query().
		Where(func(s *sql.Selector) {
			s.Where(sql.EQ("flag", constant.PolicyFlagUserAvatar))
		}).
		Count(ctx)

	if err != nil {
		log.Printf("⚠️ 警告: 检查用户头像存储策略失败: %v", err)
		return
	}

	if count > 0 {
		log.Println("✅ 用户头像存储策略已存在，跳过创建")
		return
	}

	// 获取第一个用户（管理员）作为系统目录的所有者
	firstUser, err := b.entClient.User.Query().Order(ent.Asc("id")).First(ctx)
	if err != nil {
		log.Printf("⚠️ 警告: 无法获取第一个用户，跳过创建用户头像存储策略: %v", err)
		return
	}

	// 获取或创建用户的根目录
	userRootDir, err := b.entClient.File.Query().
		Where(
			file.OwnerID(firstUser.ID),
			file.ParentIDIsNil(),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			// 创建根目录
			userRootDir, err = b.entClient.File.Create().
				SetOwnerID(firstUser.ID).
				SetName("").
				SetType(int(model.FileTypeDir)).
				Save(ctx)
			if err != nil {
				log.Printf("⚠️ 警告: 创建用户根目录失败: %v", err)
				return
			}
		} else {
			log.Printf("⚠️ 警告: 查询用户根目录失败: %v", err)
			return
		}
	}

	// 创建用户头像存储策略
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("⚠️ 警告: 无法获取当前工作目录: %v", err)
		return
	}

	avatarPath := filepath.Join(wd, constant.DefaultAvatarPolicyPath)

	// 1. 查找或创建 VFS 目录
	avatarDir, err := b.entClient.File.Query().
		Where(
			file.OwnerID(firstUser.ID),
			file.ParentID(userRootDir.ID),
			file.Name(constant.PolicyFlagUserAvatar),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			// VFS 目录不存在，创建它
			avatarDir, err = b.entClient.File.Create().
				SetOwnerID(firstUser.ID).
				SetParentID(userRootDir.ID).
				SetName(constant.PolicyFlagUserAvatar).
				SetType(int(model.FileTypeDir)).
				Save(ctx)

			if err != nil {
				log.Printf("⚠️ 警告: 创建用户头像 VFS 目录失败: %v", err)
				return
			}
			log.Printf("✅ VFS 目录 '/user_avatar' 创建成功。")
		} else {
			log.Printf("⚠️ 警告: 查询用户头像 VFS 目录失败: %v", err)
			return
		}
	} else {
		log.Printf("✅ VFS 目录 '/user_avatar' 已存在。")
	}

	// 2. 再创建策略，并关联 NodeID
	settings := model.StoragePolicySettings{
		"chunk_size":         5242880, // 5MB，头像文件通常较小
		"pre_allocate":       true,
		"upload_method":      constant.UploadMethodServer, // 头像使用服务端上传
		"allowed_extensions": []string{".jpg", ".jpeg", ".png", ".gif", ".webp"},
	}

	_, err = b.entClient.StoragePolicy.Create().
		SetName(constant.DefaultAvatarPolicyName).
		SetType(string(constant.PolicyTypeLocal)).
		SetFlag(constant.PolicyFlagUserAvatar).
		SetBasePath(avatarPath).
		SetVirtualPath("/user_avatar").
		SetMaxSize(5242880).     // 5MB 最大文件大小
		SetNodeID(avatarDir.ID). // 关联 VFS 目录节点
		SetSettings(settings).
		Save(ctx)

	if err != nil {
		log.Printf("⚠️ 警告: 创建用户头像存储策略失败: %v", err)
	} else {
		log.Printf("✅ 成功: 用户头像存储策略已创建。路径: %s", avatarPath)
	}
}

// initLinks 初始化友链、分类和标签表。
func (b *Bootstrapper) initLinks() {
	log.Println("--- 开始初始化友链模块 (Link, Category, Tag 表) ---")
	ctx := context.Background()

	count, err := b.entClient.Link.Query().Count(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 查询友链数量失败: %v", err)
		return
	}
	if count > 0 {
		log.Println("--- 友链模块已存在数据，跳过初始化。---")
		return
	}

	tx, err := b.entClient.Tx(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 启动友链初始化事务失败: %v", err)
		return
	}

	defer func() {
		if v := recover(); v != nil {
			tx.Rollback()
			panic(v)
		}
	}()

	// --- 1. 创建默认分类 ---
	catTuijian, err := tx.LinkCategory.Create().
		SetName("推荐").
		SetStyle(linkcategory.StyleCard).
		SetDescription("优秀博主，综合排序。").
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链分类 '推荐' 失败: %v", tx.Rollback())
		return
	}
	if catTuijian.ID != 1 {
		log.Printf("🔥 严重警告: '推荐' 分类创建后的 ID 不是 1 (而是 %d)。", catTuijian.ID)
	}

	// 接着创建“小伙伴”，它会自动获得 ID=2
	catShuoban, err := tx.LinkCategory.Create().
		SetName("小伙伴").
		SetStyle(linkcategory.StyleList).
		SetDescription("那些人，那些事").
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链分类 '小伙伴' 失败: %v", tx.Rollback())
		return
	}
	// 健壮性检查：确认默认分类的 ID 确实是 2
	if catShuoban.ID != 2 {
		log.Printf("🔥 严重警告: 默认分类 '小伙伴' 创建后的 ID 不是 2 (而是 %d)。申请友链的默认分类功能可能不正常。", catShuoban.ID)
	}
	log.Println("    -默认分类 '推荐' 和 '小伙伴' 创建成功。")

	// --- 2. 创建默认标签 ---
	tagTech, err := tx.LinkTag.Create().
		SetName("技术").
		SetColor("linear-gradient(38deg,#e5b085 0,#d48f16 100%)").
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链标签 '技术' 失败: %v", tx.Rollback())
		return
	}
	_, err = tx.LinkTag.Create().
		SetName("生活").
		SetColor("var(--anzhiyu-green)").
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链标签 '生活' 失败: %v", tx.Rollback())
		return
	}
	log.Println("    -默认标签 '技术' 和 '生活' 创建成功。")

	// --- 3. 创建默认友链并关联 ---
	_, err = tx.Link.Create().
		SetName("安知鱼").
		SetURL("https://blog.anheyu.com/").
		SetLogo("https://npm.elemecdn.com/anzhiyu-blog-static@1.0.4/img/avatar.jpg").
		SetDescription("生活明朗，万物可爱").
		SetSiteshot("https://npm.elemecdn.com/anzhiyu-theme-static@1.1.6/img/blog.anheyu.com.jpg"). // 添加站点快照
		SetStatus(link.StatusAPPROVED).
		SetCategoryID(catTuijian.ID). // 关联到"推荐"分类 (ID=1)
		AddTagIDs(tagTech.ID).
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链 '安知鱼' 失败: %v", tx.Rollback())
		return
	}
	log.Println("    -默认友链 '安知鱼' (卡片样式) 创建成功。")

	// 创建第二个默认友链，使用list样式的分类
	_, err = tx.Link.Create().
		SetName("安知鱼").
		SetURL("https://blog.anheyu.com/").
		SetLogo("https://npm.elemecdn.com/anzhiyu-blog-static@1.0.4/img/avatar.jpg").
		SetDescription("生活明朗，万物可爱").
		SetStatus(link.StatusAPPROVED).
		SetCategoryID(catShuoban.ID).
		AddTagIDs(tagTech.ID).
		Save(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 创建默认友链 '安知鱼' (list样式) 失败: %v", tx.Rollback())
		return
	}
	log.Println("    -默认友链 '安知鱼' (列表样式) 创建成功。")

	if err := tx.Commit(); err != nil {
		log.Printf("⚠️ 失败: 提交友链初始化事务失败: %v", err)
		return
	}

	log.Println("--- 友链模块初始化完成。---")
}

func (b *Bootstrapper) checkUserTable() {
	ctx := context.Background()
	userCount, err := b.entClient.User.Query().Count(ctx)
	if err != nil {
		log.Printf("❌ 错误: 查询 User 表记录数量失败: %v", err)
	} else if userCount == 0 {
		log.Println("User 表为空，第一个注册的用户将成为管理员。")
	}
}

// initDefaultPages 检查并初始化默认页面
func (b *Bootstrapper) initDefaultPages() {
	log.Println("--- 开始初始化默认页面 (Page 表) ---")
	ctx := context.Background()

	// 检查是否已有页面数据
	pageCount, err := b.entClient.Page.Query().Count(ctx)
	if err != nil {
		log.Printf("⚠️ 失败: 查询页面数量失败: %v", err)
		return
	}

	if pageCount > 0 {
		log.Printf("--- 页面表已有 %d 条数据，跳过默认页面初始化。---", pageCount)
		return
	}

	// 定义默认页面
	defaultPages := []struct {
		title           string
		path            string
		content         string
		markdownContent string
		description     string
		isPublished     bool
		sort            int
	}{
		{
			title: "隐私政策",
			path:  "/privacy",
			markdownContent: `协议最新更新时间为：2024-7-5

## 隐私政策

本站非常重视用户的隐私和个人信息保护。你在使用网站时，可能会收集和使用你的相关信息。通过《隐私政策》向你说明在你访问本站网站时，如何收集、使用、保存、共享和转让这些信息。

## 一、在访问时如何收集和使用你的个人信息

### 在访问时，收集访问信息的服务会收集不限于以下信息：
**网络身份标识信息**（浏览器UA、IP地址）

**设备信息**

**浏览过程**（操作方式、浏览方式与时长、性能与网络加载情况）。

## 在访问时，本人仅会处于以下目的，使用你的个人信息：

- 恶意访问识别，用于维护网站
- 恶意攻击排查，用于维护网站
- 网站点击情况监测，用于优化网站页面布局方式
- 网站加载情况监测，用于优化网站性能
- 网站访问来源及访问路径，用于网站搜索结果优化
- 网站访问请求情况，用于热度数据的展示

## 二、在评论时如何收集和使用你的个人信息

评论使用的是无登陆系统的匿名评论系统，你可以自愿填写真实的、或者虚假的信息作为你评论的展示信息。**鼓励你使用不易被人恶意识别的昵称进行评论**，但是建议你填写**真实的邮箱**以便收到回复（邮箱信息不会被公开）。

在你评论时，会额外收集你的详细个人与设备信息进行存储，用于鉴别恶意用户。

### 在访问时，本人仅会处于以下目的，收集并使用以下信息：
- 评论时会记录你的QQ账号（如果在邮箱位置填写QQ邮箱或QQ号），方便获取你的QQ头像。如果使用QQ邮箱但不想展示QQ头像，可以填写不含QQ号的QQ邮箱。（主动，存储）
- 评论时会记录你的邮箱，当我回复后会通过邮件通知你（主动，存储，不会公开邮箱）
- 评论时会记录你的网址，用于点击头像时快速进入你的网站（主动，存储）
- 评论时会记录你的IP地址，作为反垃圾的用户判别依据（被动，存储，不会公开IP，会公开IP所在城市）
- 评论会记录你的浏览器代理，用作展示系统版本、浏览器版本方便展示你使用的设备，快速定位问题（被动，存储）

## 三、如何使用 Cookies 和本地 LocalStorage 存储

本站为实现无账号评论、深色模式切换等功能，会在你的浏览器中进行本地存储，你可以随时清除浏览器中保存的所有 Cookies 以及 LocalStorage，不影响你的正常使用。

本博客中的以下业务会在你的计算机上主动存储数据：

**内置服务**

- 评论系统
- 中控台
- 胶囊音乐

关于如何使用你的Cookies，请访问[Cookie](/cookies)政策。

关于如何[在 Chrome 中清除、启用和管理 Cookie](https://support.google.com/chrome/answer/95647?co=GENIE.Platform=Desktop&hl=zh-Hans)

## 四、如何共享、转让你的个人信息

本人不会与任何公司、组织和个人共享你的隐私信息

本人不会将你的个人信息转让给任何公司、组织和个人

第三方服务的共享、转让情况详见对应服务的隐私协议

## 五、附属协议

当监测到存在恶意访问、恶意请求、恶意攻击、恶意评论的行为时，为了防止增大受害范围，可能会临时将你的ip地址及访问信息短期内添加到黑名单，短期内禁止访问。

此黑名单可能被公开，并共享给其他站点（主体并非本人）使用，包括但不限于：IP地址、设备信息、地理位置。
`,
			content: `<h2>隐私政策</h2>
<p>本站非常重视用户的隐私和个人信息保护。你在使用网站时，可能会收集和使用你的相关信息。通过《隐私政策》向你说明在你访问本站网站时，如何收集、使用、保存、共享和转让这些信息。</p>
<h2>一、在访问时如何收集和使用你的个人信息</h2>
<h3>在访问时，收集访问信息的服务会收集不限于以下信息：</h3>
<p><strong>网络身份标识信息</strong>（浏览器UA、IP地址）</p>
<p><strong>设备信息</strong></p>
<p><strong>浏览过程</strong>（操作方式、浏览方式与时长、性能与网络加载情况）。</p>
<h2>在访问时，本人仅会处于以下目的，使用你的个人信息：</h2>
<ul>
<li>恶意访问识别，用于维护网站</li>
<li>恶意攻击排查，用于维护网站</li>
<li>网站点击情况监测，用于优化网站页面布局方式</li>
<li>网站加载情况监测，用于优化网站性能</li>
<li>网站访问来源及访问路径，用于网站搜索结果优化</li>
<li>网站访问请求情况，用于热度数据的展示</li>
</ul>
<h2>二、在评论时如何收集和使用你的个人信息</h2>
<p>评论使用的是无登陆系统的匿名评论系统，你可以自愿填写真实的、或者虚假的信息作为你评论的展示信息。<strong>鼓励你使用不易被人恶意识别的昵称进行评论</strong>，但是建议你填写<strong>真实的邮箱</strong>以便收到回复（邮箱信息不会被公开）。</p>
<p>在你评论时，会额外收集你的详细个人与设备信息进行存储，用于鉴别恶意用户。</p>
<h3>在访问时，本人仅会处于以下目的，收集并使用以下信息：</h3>
<ul>
<li>评论时会记录你的QQ账号（如果在邮箱位置填写QQ邮箱或QQ号），方便获取你的QQ头像。</li>
<li>评论时会记录你的邮箱，当我回复后会通过邮件通知你（主动，存储，不会公开邮箱）</li>
<li>评论时会记录你的网址，用于点击头像时快速进入你的网站（主动，存储）</li>
<li>评论时会记录你的IP地址，作为反垃圾的用户判别依据（被动，存储，不会公开IP，会公开IP所在城市）</li>
<li>评论会记录你的浏览器代理，用作展示系统版本、浏览器版本方便展示你使用的设备，快速定位问题（被动，存储）</li>
</ul>
<h2>三、如何使用 Cookies 和本地 LocalStorage 存储</h2>
<p>本站为实现无账号评论、深色模式切换等功能，会在你的浏览器中进行本地存储，你可以随时清除浏览器中保存的所有 Cookies 以及 LocalStorage，不影响你的正常使用。</p>
<h2>四、如何共享、转让你的个人信息</h2>
<p>本人不会与任何公司、组织和个人共享你的隐私信息</p>
<p>本人不会将你的个人信息转让给任何公司、组织和个人</p>
<p>第三方服务的共享、转让情况详见对应服务的隐私协议</p>
<h2>五、附属协议</h2>
<p>当监测到存在恶意访问、恶意请求、恶意攻击、恶意评论的行为时，为了防止增大受害范围，可能会临时将你的ip地址及访问信息短期内添加到黑名单，短期内禁止访问。</p>`,
			description: "本站的隐私政策说明",
			isPublished: true,
			sort:        1,
		},
		{
			title: "Cookie 政策",
			path:  "/cookies",
			markdownContent: `本政策的最近更新日期为：2024-7-5

## Cookies

为了确保网站的可靠性、安全性和个性化，我使用 Cookies。当你接受 Cookies 时，这有助于通过识别你的身份、记住你的偏好或提供个性化用户体验来帮助我改善网站。

本政策应与我的[隐私政策](/privacy)一起阅读，该隐私政策解释了我如何使用个人信息。

如果你对我使用你的个人信息或 Cookies 的方式有任何疑问，请通过站点联系方式与我联系。

如果你想管理你的 Cookies，请按照下面"如何管理 Cookies"部分中的说明进行操作。

## 什么是 Cookies？

Cookies 是一种小型文本文件，当你访问网站时，网站可能会将这些文件放在你的计算机或设备上。

Cookies 会帮助网站或其他网站在你下次访问时识别你的设备。网站信标、像素或其他类似文件也可以做同样的事情。我在此政策中使用术语"Cookies"来指代以这种方式收集信息的所有文件。

Cookies 提供许多功能。例如，他们可以帮助我记住你喜欢深色模式还是浅色模式，分析我网站的效果。

大多数网站使用 Cookies 来收集和保留有关其访问者的个人信息。大多数 Cookies 收集一般信息，例如访问者如何到达和使用我的网站，他们使用的设备，他们的互联网协议地址（IP 地址），他们正在查看的页面及其大致位置。

## Cookies 的目的

我将Cookies分为以下类别:

| 用途 | 说明 |
| --- | --- |
| 授权 | 你访问我的网站时，我可通过 Cookie 提供正确信息，为你打造个性化的体验。 |
| 安全措施 | 我通过 Cookie 启用及支持安全功能，监控和防止可疑活动、欺诈性流量和违反版权协议的行为。 |
| 偏好、功能和服务 | 我使用功能性Cookies来让我记住你的偏好，或保存你向我提供的有关你的喜好或其他信息。 |
| 个性化广告 | 本站不涉及个性化广告服务 |
| 网站性能、分析和研究 | 我使用这些cookie来监控网站性能。这使我能够通过快速识别和解决出现的任何问题来提供高质量的体验。 |

## 如何管理Cookies？

在将Cookie放置在你的计算机或设备上之前，系统会显示一个弹出窗口，要求你同意设置这些Cookie。通过同意放置Cookies，你可以让我为你提供最佳的体验和服务。如果你愿意，你可以通过浏览器设置关闭本站的Cookie来拒绝同意放置Cookies；但是，我网站的部分功能可能无法完全或按预期运行。

以下链接提供了有关如何在所有主流浏览器中控制Cookie的说明：

- [Google Chrome](https://support.google.com/chrome/answer/95647?hl=en)
- [IE](https://support.microsoft.com/en-us/help/260971/description-of-cookies)
- [Safari（桌面版）](https://support.apple.com/guide/safari/manage-cookies-and-website-data-sfri11471/mac)
- [Safari（移动版）](https://support.apple.com/en-us/HT201265)
- [火狐浏览器](https://support.mozilla.org/en-US/kb/Cookies-information-websites-store-on-your-computer)

如你使用其他浏览器，请参阅浏览器制造商提供的文档。

## 更多信息

有关我数据处理的更多信息，请参阅我的[隐私政策](/privacy)。如果你对此Cookie政策有任何疑问，请通过站点联系方式与我联系。

## 对此Cookie政策的更改

我可能对此Cookie政策所做的任何更改都将发布在此页面上。如果更改很重要，我会在我的主页或应用上明确指出该政策已更新。
`,
			content: `<h2>Cookies</h2>
<p>为了确保网站的可靠性、安全性和个性化，我使用 Cookies。当你接受 Cookies 时，这有助于通过识别你的身份、记住你的偏好或提供个性化用户体验来帮助我改善网站。</p>
<p>本政策应与我的<a href="/privacy">隐私政策</a>一起阅读，该隐私政策解释了我如何使用个人信息。</p>
<h2>什么是 Cookies？</h2>
<p>Cookies 是一种小型文本文件，当你访问网站时，网站可能会将这些文件放在你的计算机或设备上。</p>
<p>Cookies 会帮助网站或其他网站在你下次访问时识别你的设备。Cookies 提供许多功能。例如，他们可以帮助我记住你喜欢深色模式还是浅色模式，分析我网站的效果。</p>
<h2>Cookies 的目的</h2>
<table>
<thead><tr><th>用途</th><th>说明</th></tr></thead>
<tbody>
<tr><td>授权</td><td>你访问我的网站时，我可通过 Cookie 提供正确信息，为你打造个性化的体验。</td></tr>
<tr><td>安全措施</td><td>我通过 Cookie 启用及支持安全功能，监控和防止可疑活动。</td></tr>
<tr><td>偏好、功能和服务</td><td>我使用功能性Cookies来让我记住你的偏好。</td></tr>
<tr><td>网站性能、分析和研究</td><td>我使用这些cookie来监控网站性能。</td></tr>
</tbody>
</table>
<h2>如何管理Cookies？</h2>
<p>你可以通过浏览器设置关闭本站的Cookie来拒绝同意放置Cookies；但是，我网站的部分功能可能无法完全或按预期运行。</p>
<h2>更多信息</h2>
<p>有关我数据处理的更多信息，请参阅我的<a href="/privacy">隐私政策</a>。</p>`,
			description: "本站的Cookie使用政策",
			isPublished: true,
			sort:        2,
		},
		{
			title: "版权声明",
			path:  "/copyright",
			markdownContent: `版权协议最新更新时间：2024-7-5

## 版权协议

为了保持文章质量，并保持互联网的开放共享精神，保持页面流量的稳定，综合考虑下本站的所有原创文章均采用cc协议中比较严格的署名-[非商业性使用-禁止演绎 4.0 国际标准](https://creativecommons.org/licenses/by-nc-nd/4.0/deed.en)。这篇文章主要想能够更加清楚明白的介绍本站的协议标准和要求。方便你合理的使用本站的文章。

本站无广告嵌入和商业行为。违反协议的行为不仅会损害原作者的创作热情，而且会影响整个版权环境。强烈呼吁你能够在转载时遵守协议。遵守协议的行为几乎不会对你的目标产生负面影响，鼓励创作环境是每个创作者的期望。

## 哪些文章适于本协议？

所有原创内容均在文章标题顶部，以及文章结尾的版权说明部分展示。

原创内容的非商用转载必须为完整转载且标注出处的带有` + "`完整url链接`" + `或` + "`访问原文`" + `之类字样的超链接。

作为参考资料的情况可以无需完整转载，摘录所需要的部分内容即可，但需标注出处。

## 你可以做什么？

只要你遵守本页的许可，你可以自由地共享文章的内容 — 在任何媒介以任何形式复制、发行本作品。并且无需通知作者。

## 你需要遵守什么样的许可？

### 署名

你必须标注内容的来源，你需要在文章开头部分（或者明显位置）标注原文章链接（建议使用超链接提升阅读体验）。

### 禁止商用

本站内容免费向互联网所有用户提供，分享本站文章时禁止商业性使用、禁止在转载页面中插入广告（例如谷歌广告、百度广告）、禁止阅读的拦截行为（例如关注公众号、下载App后观看文章）。

### 禁止演绎

- 作为参考资料截取部分内容：作为参考资料的情况可以无需完整转载，摘录所需要的部分内容即可，但需标注出处。
- 分享全部内容（无修改）：你需要在文章开头部分（或者明显位置）标注原文章链接（建议使用超链接）

## 分享部分截取内容或者衍生创作

目前本站全部原创文章的衍生品禁止公开分享和分发。如有更好的修改建议，可以在对应文章下留言。如有衍生创作需求，可以在评论中联系。

## 什么内容会被版权保护

包括但不限于：

- 文章封面图片
- 文章标题和正文

## 例外情况

本着友好互相进步的原则，被本站友链收录的博客允许博客文章内容的衍生品的分享和分发，但仍需标注出处。

本着互联网开放精神，你可以在博客文章下方留言要求授权博文的衍生品的分享和分发，标注你的网站地址。

禁止多篇文章的批量转载。不会在其他站点投稿。如有侵权欢迎联系站长。
`,
			content: `<h2>版权协议</h2>
<p>为了保持文章质量，并保持互联网的开放共享精神，保持页面流量的稳定，综合考虑下本站的所有原创文章均采用cc协议中比较严格的署名-<a href="https://creativecommons.org/licenses/by-nc-nd/4.0/deed.en">非商业性使用-禁止演绎 4.0 国际标准</a>。</p>
<p>本站无广告嵌入和商业行为。违反协议的行为不仅会损害原作者的创作热情，而且会影响整个版权环境。</p>
<h2>哪些文章适于本协议？</h2>
<p>所有原创内容均在文章标题顶部，以及文章结尾的版权说明部分展示。</p>
<h2>你可以做什么？</h2>
<p>只要你遵守本页的许可，你可以自由地共享文章的内容 — 在任何媒介以任何形式复制、发行本作品。并且无需通知作者。</p>
<h2>你需要遵守什么样的许可？</h2>
<h3>署名</h3>
<p>你必须标注内容的来源，你需要在文章开头部分（或者明显位置）标注原文章链接。</p>
<h3>禁止商用</h3>
<p>本站内容免费向互联网所有用户提供，分享本站文章时禁止商业性使用、禁止在转载页面中插入广告。</p>
<h3>禁止演绎</h3>
<ul>
<li>作为参考资料截取部分内容：作为参考资料的情况可以无需完整转载，摘录所需要的部分内容即可，但需标注出处。</li>
<li>分享全部内容（无修改）：你需要在文章开头部分标注原文章链接</li>
</ul>
<h2>什么内容会被版权保护</h2>
<p>包括但不限于：文章封面图片、文章标题和正文</p>
<h2>例外情况</h2>
<p>本着友好互相进步的原则，被本站友链收录的博客允许博客文章内容的衍生品的分享和分发，但仍需标注出处。</p>`,
			description: "本站的版权保护声明",
			isPublished: true,
			sort:        3,
		},
	}

	// 创建默认页面
	createdCount := 0
	for _, pageData := range defaultPages {
		_, err := b.entClient.Page.Create().
			SetTitle(pageData.title).
			SetPath(pageData.path).
			SetContent(pageData.content).
			SetMarkdownContent(pageData.markdownContent).
			SetDescription(pageData.description).
			SetIsPublished(pageData.isPublished).
			SetSort(pageData.sort).
			Save(ctx)

		if err != nil {
			log.Printf("⚠️ 失败: 创建默认页面 '%s' 失败: %v", pageData.title, err)
		} else {
			log.Printf("    -默认页面 '%s' (%s) 创建成功。", pageData.title, pageData.path)
			createdCount++
		}
	}

	if createdCount > 0 {
		log.Printf("--- 默认页面初始化完成，共创建 %d 个页面。---", createdCount)
	} else {
		log.Println("--- 默认页面初始化失败，未创建任何页面。---")
	}
}
