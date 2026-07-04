package types

import (
	"io"
)

// ServiceStatus 定义服务状态的类型
type ServiceStatus string

var ServiceStatuses = struct {
	OK    ServiceStatus
	Error ServiceStatus
}{
	OK:    "ok",
	Error: "error",
}

// PingResponse 是 /ping 端点返回的 JSON 结构
type PingResponse struct {
	Status        ServiceStatus `json:"status"`
	Version       string        `json:"version"`     // 应用版本号
	InstanceID    string        `json:"instance_id"` // 本次启动的唯一实例ID
	ServerDirPath string        `json:"server_dir"`  // 主服务映射的本地绝对路径
	WebdavDirPath string        `json:"webdav_dir"`  // WebDAV 服务映射的本地绝对路径 (如果启用)
}

// FFProbeRawMetadata 用于直接解析 ffprobe 的 JSON 输出
// 它可以完整解析 `ffprobe -show_format -show_streams -show_chapters` 命令的输出
type FFProbeRawMetadata struct {
	Streams []struct {
		// --- 通用流信息 ---
		Index          int    `json:"index"`
		CodecName      string `json:"codec_name"`      // 如 "h264", "aac"
		CodecLongName  string `json:"codec_long_name"` // 如 "H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10"
		CodecType      string `json:"codec_type"`      // "video", "audio", "subtitle"
		CodecTimeBase  string `json:"codec_time_base"`
		CodecTagString string `json:"codec_tag_string"`
		CodecTag       string `json:"codec_tag"`
		RFrameRate     string `json:"r_frame_rate"`   // 实际帧率, 如 "25/1"
		AvgFrameRate   string `json:"avg_frame_rate"` // 平均帧率, 如 "25/1"
		TimeBase       string `json:"time_base"`
		StartPts       int64  `json:"start_pts"`
		StartTime      string `json:"start_time"`
		DurationTs     int64  `json:"duration_ts"`
		Duration       string `json:"duration"`
		BitRate        string `json:"bit_rate"`  // 比特率
		NbFrames       string `json:"nb_frames"` // 帧数
		Disposition    struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
		} `json:"disposition"`
		Tags map[string]string `json:"tags"` // 流级别的标签，如 'language', 'title'

		// --- 视频流特有信息 ---
		Width              int    `json:"width"`
		Height             int    `json:"height"`
		CodedWidth         int    `json:"coded_width"`
		CodedHeight        int    `json:"coded_height"`
		HasBFrames         int    `json:"has_b_frames"`
		SampleAspectRatio  string `json:"sample_aspect_ratio"`  // 如 "1:1"
		DisplayAspectRatio string `json:"display_aspect_ratio"` // 如 "16:9"
		PixFmt             string `json:"pix_fmt"`              // 像素格式, 如 "yuv420p"
		Level              int    `json:"level"`
		ColorRange         string `json:"color_range"`     // 如 "tv", "pc"
		ColorSpace         string `json:"color_space"`     // 如 "bt709"
		ColorTransfer      string `json:"color_transfer"`  // 如 "bt709"
		ColorPrimaries     string `json:"color_primaries"` // 如 "bt709"
		ChromaLocation     string `json:"chroma_location"` // 如 "left"
		FieldOrder         string `json:"field_order"`     // 如 "progressive"

		// --- 音频流特有信息 ---
		SampleFmt     string `json:"sample_fmt"`     // 采样格式, 如 "fltp", "s16"
		SampleRate    string `json:"sample_rate"`    // 采样率, 如 "48000"
		Channels      int    `json:"channels"`       // 声道数
		ChannelLayout string `json:"channel_layout"` // 声道布局, 如 "stereo"
		BitsPerSample int    `json:"bits_per_sample"`
	} `json:"streams"`
	Format struct {
		NBStreams      int               `json:"nb_streams"`
		NBPrograms     int               `json:"nb_programs"`
		FormatName     string            `json:"format_name"`      // 容器格式, 如 "mov,mp4,m4a,3gp,3g2,mj2"
		FormatLongName string            `json:"format_long_name"` // 如 "QuickTime / MOV"
		StartTime      string            `json:"start_time"`
		Duration       string            `json:"duration"` // 总时长
		Size           string            `json:"size"`     // 文件大小（字节）
		BitRate        string            `json:"bit_rate"` // 总比特率
		ProbeScore     int               `json:"probe_score"`
		Tags           map[string]string `json:"tags"` // 全局标签, 如 'title', 'encoder'
	} `json:"format"`
	// 注意：Chapters 部分在单独使用 -show_chapters 时结构不同，这里是为组合命令准备的
	Chapters []struct {
		ID       int64  `json:"id"`
		TimeBase string `json:"time_base"`
		Start    int64  `json:"start"`
		End      int64  `json:"end"`
		Tags     struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
}

// type FFProbeRawMetadata struct {
// 	Format struct {
// 		Duration string `json:"duration"`
// 	} `json:"format"`
// 	Streams []struct {
// 		CodecType string `json:"codec_type"`
// 		Width     int    `json:"width"`
// 		Height    int    `json:"height"`
// 	} `json:"streams"`
// }

// SubChunkInfo 存储子分片的元数据
type SubChunkInfo struct {
	Index    int    `json:"index"`    // 子分片的序号 (2, 3, 4...)
	Filename string `json:"filename"` // 子分片的文件名
	Size     int64  `json:"size"`     // 子分片大小
	MD5      string `json:"md5"`      // 子分片内容的 MD5 哈希
	// 【新增字段】记录该子分片在完整加密文件中的起始字节偏移量
	Offset int64 `json:"offset"`
}

// 所有扩展名都不带 .
type BinExtGroup struct {
	// 文本类加密容器的扩展名。
	Text string `json:"text"`
	// 图像类加密容器的扩展名。
	Image string `json:"image"`
	// 音频类加密容器的扩展名。
	Audio string `json:"audio"`
	// 视频类加密容器的扩展名。
	Video string `json:"video"`
	//word ppt excel 类加密容器的扩展名。
	WPS string `json:"wps"`
	// pdf 类加密容器的扩展名。
	PDF string `json:"pdf"`
}

// --- WebDAV 服务器设置 ---
type WebdavServer struct {
	// 路由（例如 /webdav/）
	Root string `json:"root"`
	// 文件系统的根目录（例如 /path/to/your/files），而不是 WebDAV 的路由前缀（例如 /webdav/）
	Dir string `json:"dir"`
	// WebDAV 基础认证的用户名
	Username string `json:"username"`
	// WebDAV 基础认证的密码
	Password string `json:"password"`
}

// --- 内置HTTP服务器 设置 ---
type HttpServer struct {
	// encv HTTP 服务器的端口，请不要填写 encv Webdav Server、 OpenList 或其他已使用的端口
	Port int `json:"port"`
	// 文件系统的根目录（例如 /path/to/your/files），支持相对路径
	Dir string `json:"dir"`
}

// --- 管理后台服务器 设置 ---
type AdminServer struct {
	// 管理员密码，留空则禁用登录
	Password string `json:"password"`
}

// --- Openlist 代理服务器设置 ---
type OpenlistProxyServer struct {
	Sites map[string]ProxySiteConfig `json:"sites"`
	// 禁用签名，目前没发现这个值有什么影响
	DisableSignatureVerification bool `json:"disable_signature_verification"`
}

type ProxySiteConfig struct {
	// OpenList 的主机，一般是 IP 或者域名，加上端口号，比如 localhost:5244
	Host string `json:"host"`
	// 站点描述，可选，用于用户自己标识区分
	Description string `json:"description,omitempty"`
	// Token前端输入
	// BuiltIn 标记该站点由 encv-go 运行时自动注册（例如本地 OpenList 插件）
	// 持久化时会被跳过，列表展示时会排在最前
	BuiltIn bool `json:"built_in,omitempty"`
}

// --- 日志配置 ---
type LogConfig struct {
	// 日志级别: debug, info, warn, error
	// 设置为 debug 可以看到详细的插件匹配、文件处理等调试信息。
	// info 级别会输出关键操作信息和服务状态。
	// warn 输出潜在问题但不影响运行的警告。
	// error 只输出错误信息。
	Level string `json:"level"`
	// 日志文件路径，为空则只输出到控制台
	// 如果指定了文件路径，控制台输出带颜色的文本格式，文件输出 JSON 格式。
	// 支持相对路径（相对于可执行文件）和绝对路径。
	// 示例: "encv.log" 或 "D:/logs/encv.log"
	File string `json:"file"`
}

// MobileConfig 移动端专用配置段，桌面端忽略。
// 字段命名镜像目标配置路径，实现无歧义映射：
//
//	mobile.server.dir   → server.dir
//	mobile.output.path  → output_path
//	mobile.webdav.dir   → webdav.dir
//
// 此配置段是运行时 overlay（覆盖层），不修改持久化的 config.user.json。
// Go 端 Load() finalize() 阶段自动应用，Android 端通过 ENCV_MOBILE=1 触发。
type MobileConfig struct {
	Server *MobileServerConfig `json:"server,omitempty"`
	Output *MobileOutputConfig `json:"output,omitempty"`
	Webdav *MobileWebdavConfig `json:"webdav,omitempty"`
}

type MobileServerConfig struct {
	Dir string `json:"dir"` // 覆盖 server.dir
}

type MobileOutputConfig struct {
	Path string `json:"path"` // 覆盖 output_path
}

type MobileWebdavConfig struct {
	Dir string `json:"dir"` // 覆盖 webdav.dir
}

// DecryptedContent 包含解密后的所有内容
type DecryptedContent struct {
	Index      Index
	DataStream io.ReadCloser
}
