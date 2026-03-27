package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weinaike/dubsync/src"
	"gopkg.in/yaml.v3"
)

// FileTestConfig 文件测试配置格式
type FileTestConfig struct {
	Name          string            `json:"name" yaml:"name"`
	Mode          string            `json:"mode" yaml:"mode"`                       // "A" or "B"
	TotalDuration int               `json:"totalDuration" yaml:"totalDuration"`     // 毫秒
	Segments      []FileSegmentSpec `json:"segments" yaml:"segments"`
}

// FileSegmentSpec 片段规格
type FileSegmentSpec struct {
	OrigStartMs int `json:"origStartMs" yaml:"origStartMs"` // 原始起始时间（毫秒）
	OrigEndMs   int `json:"origEndMs" yaml:"origEndMs"`     // 原始结束时间（毫秒）
	TransLenMs  int `json:"transLenMs" yaml:"transLenMs"`   // 翻译长度（毫秒）
}

// NamedTestCase 带名称的测试用例
type NamedTestCase struct {
	Name  string
	Input *timeline.PipelineInput
}

// LoadFromJSON 从 JSON 文件加载测试数据
func LoadFromJSON(path string) (*NamedTestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var config FileTestConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	input, err := fileConfigToInput(&config)
	if err != nil {
		return nil, err
	}

	return &NamedTestCase{
		Name:  config.Name,
		Input: input,
	}, nil
}

// LoadFromYAML 从 YAML 文件加载测试数据
func LoadFromYAML(path string) (*NamedTestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var config FileTestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	input, err := fileConfigToInput(&config)
	if err != nil {
		return nil, err
	}

	return &NamedTestCase{
		Name:  config.Name,
		Input: input,
	}, nil
}

// LoadAllFromDir 加载目录下所有测试文件
// ext 为文件扩展名，如 ".json" 或 ".yaml"
func LoadAllFromDir(dir string, ext string) ([]NamedTestCase, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*"+ext))
	if err != nil {
		return nil, fmt.Errorf("glob files: %w", err)
	}

	cases := make([]NamedTestCase, 0, len(files))
	for _, file := range files {
		var tc *NamedTestCase
		var err error

		switch ext {
		case ".json":
			tc, err = LoadFromJSON(file)
		case ".yaml", ".yml":
			tc, err = LoadFromYAML(file)
		default:
			return nil, fmt.Errorf("unsupported extension: %s", ext)
		}

		if err != nil {
			return nil, fmt.Errorf("load %s: %w", file, err)
		}
		cases = append(cases, *tc)
	}

	return cases, nil
}

// fileConfigToInput 将文件配置转换为 PipelineInput
func fileConfigToInput(config *FileTestConfig) (*timeline.PipelineInput, error) {
	// 解析模式
	var mode timeline.Mode
	switch strings.ToUpper(config.Mode) {
	case "A", "MODEA", "VIDEO":
		mode = timeline.ModeA
	case "B", "MODEB", "PODCAST":
		mode = timeline.ModeB
	default:
		return nil, fmt.Errorf("invalid mode: %s", config.Mode)
	}

	// 构建片段
	sources := make([]timeline.SourceSegment, len(config.Segments))
	tts := make([]timeline.TTSSegment, len(config.Segments))

	for i, spec := range config.Segments {
		sources[i] = timeline.SourceSegment{
			Index: i,
			Interval: timeline.TimeInterval{
				Start: time.Duration(spec.OrigStartMs) * time.Millisecond,
				End:   time.Duration(spec.OrigEndMs) * time.Millisecond,
			},
		}
		tts[i] = timeline.TTSSegment{
			Index:      i,
			NaturalLen: time.Duration(spec.TransLenMs) * time.Millisecond,
		}
	}

	// 创建配置
	cfg := timeline.NewDefaultConfig(mode)
	cfg.TotalDuration = time.Duration(config.TotalDuration) * time.Millisecond

	return &timeline.PipelineInput{
		SourceSegments: sources,
		TTSSegments:    tts,
		Config:         *cfg,
	}, nil
}
