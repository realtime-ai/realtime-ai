package audio

import (
	"fmt"

	"github.com/asticode/go-astiav"
)

type Resample struct {
	ctx       *astiav.SoftwareResampleContext
	inFrame   *astiav.Frame
	outFrame  *astiav.Frame
	inLayout  astiav.ChannelLayout
	outLayout astiav.ChannelLayout
	inRate    int
	outRate   int
}

// NewResample 创建新的重采样器
func NewResample(inRate, outRate int, inLayout, outLayout astiav.ChannelLayout) (*Resample, error) {
	// 验证参数
	if inRate <= 0 {
		return nil, fmt.Errorf("invalid input sample rate: %d", inRate)
	}
	if outRate <= 0 {
		return nil, fmt.Errorf("invalid output sample rate: %d", outRate)
	}

	r := &Resample{
		inRate:    inRate,
		outRate:   outRate,
		inLayout:  inLayout,
		outLayout: outLayout,
	}

	// 创建重采样上下文
	r.ctx = astiav.AllocSoftwareResampleContext()
	if r.ctx == nil {
		return nil, fmt.Errorf("failed to allocate resample context")
	}

	// 分配输入帧
	r.inFrame = astiav.AllocFrame()
	if r.inFrame == nil {
		r.Free()
		return nil, fmt.Errorf("failed to allocate input frame")
	}

	// 分配输出帧
	r.outFrame = astiav.AllocFrame()
	if r.outFrame == nil {
		r.Free()
		return nil, fmt.Errorf("failed to allocate output frame")
	}

	return r, nil
}

// Free 释放资源
func (r *Resample) Free() {
	if r.ctx != nil {
		r.ctx.Free()
		r.ctx = nil
	}
	if r.inFrame != nil {
		r.inFrame.Free()
		r.inFrame = nil
	}
	if r.outFrame != nil {
		r.outFrame.Free()
		r.outFrame = nil
	}
}

// Resample 执行音频重采样
func (r *Resample) Resample(inputData []byte) ([]byte, error) {
	const align = 0

	// 检查输入数据
	if len(inputData) == 0 {
		return nil, fmt.Errorf("empty input data")
	}

	// 计算每个采样的字节数
	bytesPerSample := 2 // S16 格式为 2 字节
	var inChannels int
	if r.inLayout == astiav.ChannelLayoutMono {
		inChannels = 1
	} else if r.inLayout == astiav.ChannelLayoutStereo {
		inChannels = 2
	} else {
		return nil, fmt.Errorf("unsupported channel layout")
	}
	bytesPerFrame := bytesPerSample * inChannels

	// 计算采样点数
	numSamples := len(inputData) / bytesPerFrame
	if numSamples == 0 {
		return nil, fmt.Errorf("input data too small")
	}

	// 释放之前的帧缓冲区
	r.inFrame.Unref()
	r.outFrame.Unref()

	// 设置输入帧参数
	r.inFrame.SetChannelLayout(r.inLayout)
	r.inFrame.SetSampleFormat(astiav.SampleFormatS16)
	r.inFrame.SetSampleRate(r.inRate)
	r.inFrame.SetNbSamples(numSamples)

	// 设置输出帧参数
	r.outFrame.SetChannelLayout(r.outLayout)
	r.outFrame.SetSampleFormat(astiav.SampleFormatS16)
	r.outFrame.SetSampleRate(r.outRate)

	// 计算输出采样点数，考虑采样率转换
	outNumSamples := (numSamples * r.outRate) / r.inRate
	if outNumSamples == 0 {
		outNumSamples = 1
	}
	r.outFrame.SetNbSamples(outNumSamples)

	// 分配输入帧缓冲区
	if err := r.inFrame.AllocBuffer(align); err != nil {
		return nil, fmt.Errorf("failed to allocate input buffer: %w", err)
	}

	// 分配输出帧缓冲区
	if err := r.outFrame.AllocBuffer(align); err != nil {
		return nil, fmt.Errorf("failed to allocate output buffer: %w", err)
	}

	// 确保输入帧可写
	if err := r.inFrame.MakeWritable(); err != nil {
		return nil, fmt.Errorf("making frame writable failed: %w", err)
	}

	// FFmpeg 可能需要更大的对齐缓冲区，获取实际缓冲区大小
	actualBufferSize, err := r.inFrame.SamplesBufferSize(align)
	if err != nil {
		return nil, fmt.Errorf("failed to get buffer size: %w", err)
	}

	// 如果输入数据小于实际缓冲区大小，需要填充零
	inputBuffer := inputData
	if len(inputData) < actualBufferSize {
		inputBuffer = make([]byte, actualBufferSize)
		copy(inputBuffer, inputData)
	}

	// 设置输入数据
	if err := r.inFrame.Data().SetBytes(inputBuffer[:actualBufferSize], align); err != nil {
		return nil, fmt.Errorf("setting frame's data failed: %w", err)
	}

	// 执行重采样
	if err := r.ctx.ConvertFrame(r.inFrame, r.outFrame); err != nil {
		return nil, fmt.Errorf("failed to resample: %w", err)
	}

	// 获取输出数据
	outputData, err := r.outFrame.Data().Bytes(align)
	if err != nil {
		return nil, fmt.Errorf("getting output data failed: %w", err)
	}

	return outputData, nil
}
