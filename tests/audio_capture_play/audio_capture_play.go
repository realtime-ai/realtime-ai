package main

import (
	"fmt"
	"os"

	"github.com/gen2brain/malgo"
)

func main() {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		fmt.Printf("LOG <%v>\n", message)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = 48000
	deviceConfig.Alsa.NoMMap = 1

	var playbackSampleCount uint32
	var capturedSampleCount uint32
	pCapturedSamples := make([]byte, 0)

	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {

		sampleCount := framecount * deviceConfig.Capture.Channels * sizeInBytes

		newCapturedSampleCount := capturedSampleCount + sampleCount

		pCapturedSamples = append(pCapturedSamples, pSample...)

		capturedSampleCount = newCapturedSampleCount

	}

	fmt.Println("Recording...")
	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, captureCallbacks)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = device.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Press Enter to stop recording...")
	fmt.Scanln()

	device.Uninit()

	deviceConfig = malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.SampleRate = 48000
	deviceConfig.Alsa.NoMMap = 1
	deviceConfig.PeriodSizeInMilliseconds = 20
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 1

	onSendFrames := func(pSample, nil []byte, framecount uint32) {
		samplesToRead := framecount * deviceConfig.Playback.Channels * sizeInBytes
		if samplesToRead > capturedSampleCount-playbackSampleCount {
			samplesToRead = capturedSampleCount - playbackSampleCount
		}

		copy(pSample, pCapturedSamples[playbackSampleCount:playbackSampleCount+samplesToRead])

		playbackSampleCount += samplesToRead

		if playbackSampleCount == uint32(len(pCapturedSamples)) {
			playbackSampleCount = 0
		}
	}

	fmt.Println("Playing...")
	playbackCallbacks := malgo.DeviceCallbacks{
		Data: onSendFrames,
	}

	device, err = malgo.InitDevice(ctx.Context, deviceConfig, playbackCallbacks)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = device.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Press Enter to quit...")
	fmt.Scanln()

	device.Uninit()
}
