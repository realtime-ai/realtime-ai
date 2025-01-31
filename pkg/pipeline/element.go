package pipeline

import (
	"context"
	"fmt"
	"reflect"
)

// PropertyDesc 用来描述一个属性的元信息，如类型、可读可写等
type PropertyDesc struct {
	Name     string
	Type     reflect.Type
	Writable bool
	Readable bool
	Default  interface{}
}

type Element interface {
	Init(ctx context.Context) error
	In() chan<- *PipelineMessage
	Out() <-chan *PipelineMessage
	Start(ctx context.Context) error
	Stop() error

	SetBus(bus Bus)
	SetProperty(name string, value interface{}) error
	GetProperty(name string) (interface{}, error)
}

type BaseElement struct {
	propertyDescs map[string]PropertyDesc // 保存此元素“可用属性”的描述信息
	properties    map[string]interface{}  // 保存此元素“当前属性值”
	bus           Bus

	InChan  chan *PipelineMessage
	OutChan chan *PipelineMessage
}

func NewBaseElement(bufferSize int) *BaseElement {
	return &BaseElement{
		InChan:        make(chan *PipelineMessage, bufferSize),
		OutChan:       make(chan *PipelineMessage, bufferSize),
		propertyDescs: make(map[string]PropertyDesc),
		properties:    make(map[string]interface{}),
	}
}

func (b *BaseElement) Init(ctx context.Context) error {
	return nil
}

func (b *BaseElement) In() chan<- *PipelineMessage {
	return b.InChan
}

func (b *BaseElement) Out() <-chan *PipelineMessage {
	return b.OutChan
}

func (b *BaseElement) Start(ctx context.Context) error {
	return nil // 具体逻辑由子结构实现
}

func (b *BaseElement) Stop() error {
	return nil
}

func (b *BaseElement) SetBus(bus Bus) {
	b.bus = bus
}

func (b *BaseElement) RegisterProperty(desc PropertyDesc) error {
	if _, exists := b.propertyDescs[desc.Name]; exists {
		return fmt.Errorf("property %s already registered", desc.Name)
	}
	b.propertyDescs[desc.Name] = desc
	// 同时初始化其默认值
	b.properties[desc.Name] = desc.Default
	return nil
}

func (b *BaseElement) SetProperty(name string, value interface{}) error {
	desc, ok := b.propertyDescs[name]
	if !ok {
		return fmt.Errorf("unknown property %q", name)
	}
	if !desc.Writable {
		return fmt.Errorf("property %q is not writable", name)
	}
	// 类型检查
	if reflect.TypeOf(value) != desc.Type {
		return fmt.Errorf(
			"property %q expects type %v, but got %v",
			name, desc.Type, reflect.TypeOf(value),
		)
	}
	b.properties[name] = value
	return nil
}

func (b *BaseElement) GetProperty(name string) (interface{}, error) {
	desc, ok := b.propertyDescs[name]
	if !ok {
		return nil, fmt.Errorf("unknown property %q", name)
	}
	if !desc.Readable {
		return nil, fmt.Errorf("property %q is not readable", name)
	}
	value := b.properties[name]
	return value, nil
}
