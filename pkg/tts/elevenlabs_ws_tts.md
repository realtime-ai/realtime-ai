
# WebSocket

GET /v1/text-to-speech/{voice_id}/stream-input

The Text-to-Speech WebSockets API is designed to generate audio from partial text input
while ensuring consistency throughout the generated audio. Although highly flexible,
the WebSockets API isn't a one-size-fits-all solution. It's well-suited for scenarios where:
  * The input text is being streamed or generated in chunks.
  * Word-to-audio alignment information is required.

However, it may not be the best choice when:
  * The entire input text is available upfront. Given that the generations are partial,
    some buffering is involved, which could potentially result in slightly higher latency compared
    to a standard HTTP request.
  * You want to quickly experiment or prototype. Working with WebSockets can be harder and more
    complex than using a standard HTTP API, which might slow down rapid development and testing.


Reference: https://elevenlabs.io/docs/api-reference/text-to-speech/v-1-text-to-speech-voice-id-stream-input

## AsyncAPI Specification

```yaml
asyncapi: 2.6.0
info:
  title: V 1 Text To Speech Voice Id Stream Input
  version: subpackage_v1TextToSpeechVoiceIdStreamInput.v1TextToSpeechVoiceIdStreamInput
  description: >
    The Text-to-Speech WebSockets API is designed to generate audio from partial
    text input

    while ensuring consistency throughout the generated audio. Although highly
    flexible,

    the WebSockets API isn't a one-size-fits-all solution. It's well-suited for
    scenarios where:
      * The input text is being streamed or generated in chunks.
      * Word-to-audio alignment information is required.

    However, it may not be the best choice when:
      * The entire input text is available upfront. Given that the generations are partial,
        some buffering is involved, which could potentially result in slightly higher latency compared
        to a standard HTTP request.
      * You want to quickly experiment or prototype. Working with WebSockets can be harder and more
        complex than using a standard HTTP API, which might slow down rapid development and testing.
channels:
  /v1/text-to-speech/{voice_id}/stream-input:
    description: >
      The Text-to-Speech WebSockets API is designed to generate audio from
      partial text input

      while ensuring consistency throughout the generated audio. Although highly
      flexible,

      the WebSockets API isn't a one-size-fits-all solution. It's well-suited
      for scenarios where:
        * The input text is being streamed or generated in chunks.
        * Word-to-audio alignment information is required.

      However, it may not be the best choice when:
        * The entire input text is available upfront. Given that the generations are partial,
          some buffering is involved, which could potentially result in slightly higher latency compared
          to a standard HTTP request.
        * You want to quickly experiment or prototype. Working with WebSockets can be harder and more
          complex than using a standard HTTP API, which might slow down rapid development and testing.
    parameters:
      voice_id:
        description: The unique identifier for the voice to use in the TTS process.
        schema:
          type: string
    bindings:
      ws:
        query:
          type: object
          properties:
            authorization:
              type: string
            single_use_token:
              type: string
            model_id:
              type: string
            language_code:
              type: string
            enable_logging:
              type: boolean
              default: true
            enable_ssml_parsing:
              type: boolean
              default: false
            output_format:
              $ref: '#/components/schemas/output_format'
            inactivity_timeout:
              type: integer
              default: 20
            sync_alignment:
              type: boolean
              default: false
            auto_mode:
              type: boolean
              default: false
            apply_text_normalization:
              $ref: '#/components/schemas/apply_text_normalization'
            seed:
              type: integer
        headers:
          type: object
          properties:
            xi-api-key:
              type: string
    publish:
      operationId: v-1-text-to-speech-voice-id-stream-input-publish
      summary: subscribe
      description: Receive messages from the WebSocket
      message:
        name: subscribe
        title: subscribe
        description: Receive messages from the WebSocket
        payload:
          $ref: '#/components/schemas/V1TextToSpeechVoiceIdStreamInputSubscribe'
    subscribe:
      operationId: v-1-text-to-speech-voice-id-stream-input-subscribe
      summary: publish
      description: Send messages to the WebSocket
      message:
        name: publish
        title: publish
        description: Send messages to the WebSocket
        payload:
          $ref: '#/components/schemas/V1TextToSpeechVoiceIdStreamInputPublish'
servers:
  Production:
    url: wss://api.elevenlabs.io/
    protocol: wss
    x-default: true
  Production US:
    url: wss://api.us.elevenlabs.io/
    protocol: wss
  Production EU:
    url: wss://api.eu.residency.elevenlabs.io/
    protocol: wss
  Production India:
    url: wss://api.in.residency.elevenlabs.io/
    protocol: wss
components:
  schemas:
    output_format:
      type: string
      enum:
        - value: mp3_22050_32
        - value: mp3_44100_32
        - value: mp3_44100_64
        - value: mp3_44100_96
        - value: mp3_44100_128
        - value: mp3_44100_192
        - value: pcm_8000
        - value: pcm_16000
        - value: pcm_22050
        - value: pcm_24000
        - value: pcm_44100
        - value: ulaw_8000
        - value: alaw_8000
        - value: opus_48000_32
        - value: opus_48000_64
        - value: opus_48000_96
        - value: opus_48000_128
        - value: opus_48000_192
    apply_text_normalization:
      type: string
      enum:
        - value: auto
        - value: 'on'
        - value: 'off'
      default: auto
    NormalizedAlignment:
      type: object
      properties:
        charStartTimesMs:
          type: array
          items:
            type: integer
          description: >
            A list of starting times (in milliseconds) for each character in the
            normalized text as it

            corresponds to the audio. For instance, the character 'H' starts at
            time 0 ms in the audio.

            Note these times are relative to the returned chunk from the model,
            and not the

            full audio response.
        charDurationsMs:
          type: array
          items:
            type: integer
          description: >
            A list of durations (in milliseconds) for each character in the
            normalized text as it

            corresponds to the audio. For instance, the character 'H' lasts for
            3 ms in the audio.

            Note these times are relative to the returned chunk from the model,
            and not the

            full audio response.
        chars:
          type: array
          items:
            type: string
          description: >
            A list of characters in the normalized text sequence. For instance,
            the first character is 'H'.

            Note that this list may contain spaces, punctuation, and other
            special characters.

            The length of this list should be the same as the lengths of
            `charStartTimesMs` and `charDurationsMs`.
    Alignment:
      type: object
      properties:
        charStartTimesMs:
          type: array
          items:
            type: integer
          description: >
            A list of starting times (in milliseconds) for each character in the
            text as it

            corresponds to the audio. For instance, the character 'H' starts at
            time 0 ms in the audio.

            Note these times are relative to the returned chunk from the model,
            and not the

            full audio response.
        charDurationsMs:
          type: array
          items:
            type: integer
          description: >
            A list of durations (in milliseconds) for each character in the text
            as it

            corresponds to the audio. For instance, the character 'H' lasts for
            3 ms in the audio.

            Note these times are relative to the returned chunk from the model,
            and not the

            full audio response.
        chars:
          type: array
          items:
            type: string
          description: >
            A list of characters in the text sequence. For instance, the first
            character is 'H'.

            Note that this list may contain spaces, punctuation, and other
            special characters.

            The length of this list should be the same as the lengths of
            `charStartTimesMs` and `charDurationsMs`.
    AudioOutput:
      type: object
      properties:
        audio:
          type: string
          description: >
            A generated partial audio chunk, encoded using the selected
            output_format, by default this

            is MP3 encoded as a base64 string.
        normalizedAlignment:
          $ref: '#/components/schemas/NormalizedAlignment'
        alignment:
          $ref: '#/components/schemas/Alignment'
      required:
        - audio
    FinalOutput:
      type: object
      properties:
        isFinal:
          type: string
          enum:
            - type: booleanLiteral
              value: true
          description: >
            Indicates if the generation is complete. If set to `True`, `audio`
            will be null.
    V1TextToSpeechVoiceIdStreamInputSubscribe:
      oneOf:
        - $ref: '#/components/schemas/AudioOutput'
        - $ref: '#/components/schemas/FinalOutput'
    RealtimeVoiceSettings:
      type: object
      properties:
        stability:
          type: number
          format: double
          default: 0.5
          description: Defines the stability for voice settings.
        similarity_boost:
          type: number
          format: double
          default: 0.75
          description: Defines the similarity boost for voice settings.
        style:
          type: number
          format: double
          default: 0
          description: >-
            Defines the style for voice settings. This parameter is available on
            V2+ models.
        use_speaker_boost:
          type: boolean
          default: true
          description: >-
            Defines the use speaker boost for voice settings. This parameter is
            available on V2+ models.
        speed:
          type: number
          format: double
          default: 1
          description: >-
            Controls the speed of the generated speech. Values range from 0.7 to
            1.2, with 1.0 being the default speed.
    GenerationConfig:
      type: object
      properties:
        chunk_length_schedule:
          type: array
          items:
            type: number
            format: double
          description: >
            This is an advanced setting that most users shouldn't need to use.
            It relates to our

            generation schedule.


            Our WebSocket service incorporates a buffer system designed to
            optimize the Time To First Byte (TTFB) while maintaining
            high-quality streaming.


            All text sent to the WebSocket endpoint is added to this buffer and
            only when that buffer reaches a certain size is an audio generation
            attempted. This is because our model provides higher quality audio
            when the model has longer inputs, and can deduce more context about
            how the text should be delivered.


            The buffer ensures smooth audio data delivery and is automatically
            emptied with a final audio generation either when the stream is
            closed, or upon sending a `flush` command. We have advanced settings
            for changing the chunk schedule, which can improve latency at the
            cost of quality by generating audio more frequently with smaller
            text inputs.


            The `chunk_length_schedule` determines the minimum amount of text
            that needs to be sent and present in our

            buffer before audio starts being generated. This is to maximise the
            amount of context available to

            the model to improve audio quality, whilst balancing latency of the
            returned audio chunks.


            The default value for `chunk_length_schedule` is: [120, 160, 250,
            290].


            This means that the first chunk of audio will not be generated until
            you send text that

            totals at least 120 characters long. The next chunk of audio will
            only be generated once a

            further 160 characters have been sent. The third audio chunk will be
            generated after the

            next 250 characters. Then the fourth, and beyond, will be generated
            in sets of at least 290 characters.


            Customize this array to suit your needs. If you want to generate
            audio more frequently

            to optimise latency, you can reduce the values in the array. Note
            that setting the values

            too low may result in lower quality audio. Please test and adjust as
            needed.


            Each item should be in the range 50-500.
    PronunciationDictionaryLocator:
      type: object
      properties:
        pronunciation_dictionary_id:
          type: string
          description: The unique identifier of the pronunciation dictionary
        version_id:
          type: string
          description: The version identifier of the pronunciation dictionary
      required:
        - pronunciation_dictionary_id
        - version_id
    InitializeConnection:
      type: object
      properties:
        text:
          type: string
          enum:
            - type: stringLiteral
              value: ' '
          description: The initial text that must be sent is a blank space.
        voice_settings:
          $ref: '#/components/schemas/RealtimeVoiceSettings'
        generation_config:
          $ref: '#/components/schemas/GenerationConfig'
        pronunciation_dictionary_locators:
          type: array
          items:
            $ref: '#/components/schemas/PronunciationDictionaryLocator'
          description: >
            Optional list of pronunciation dictionary locators. If provided,
            these dictionaries will be used to

            modify pronunciation of matching text. Must only be provided in the
            first message.


            Note: Pronunciation dictionary matches will only be respected within
            a provided chunk.
        xi-api-key:
          type: string
          description: >
            Your ElevenLabs API key. This can only be included in the first
            message and is not needed if present in the header.
        authorization:
          type: string
          description: >
            Your authorization bearer token. This can only be included in the
            first message and is not needed if present in the header.
      required:
        - text
    SendText:
      type: object
      properties:
        text:
          type: string
          description: >-
            The text to be sent to the API for audio generation. Should always
            end with a single space string.
        try_trigger_generation:
          type: boolean
          default: false
          description: >
            This is an advanced setting that most users shouldn't need to use.
            It relates to our generation schedule.


            Use this to attempt to immediately trigger the generation of audio,
            overriding the `chunk_length_schedule`.

            Unlike flush, `try_trigger_generation` will only generate audio if
            our

            buffer contains more than a minimum

            threshold of characters, this is to ensure a higher quality response
            from our model.


            Note that overriding the chunk schedule to generate small amounts of

            text may result in lower quality audio, therefore, only use this
            parameter if you

            really need text to be processed immediately. We generally recommend
            keeping the default value of

            `false` and adjusting the `chunk_length_schedule` in the
            `generation_config` instead.
        voice_settings:
          $ref: '#/components/schemas/RealtimeVoiceSettings'
          description: >-
            The voice settings field can be provided in the first
            `InitializeConnection` message and then must either be not provided
            or not changed.
        generator_config:
          $ref: '#/components/schemas/GenerationConfig'
          description: >-
            The generator config field can be provided in the first
            `InitializeConnection` message and then must either be not provided
            or not changed.
        flush:
          type: boolean
          default: false
          description: >
            Flush forces the generation of audio. Set this value to true when
            you have finished sending text, but want to keep the websocket
            connection open.


            This is useful when you want to ensure that the last chunk of audio
            is generated even when the length of text sent is smaller than the
            value set in chunk_length_schedule (e.g. 120 or 50).
      required:
        - text
    CloseConnection:
      type: object
      properties:
        text:
          type: string
          enum:
            - type: stringLiteral
              value: ''
          description: End the stream with an empty string
      required:
        - text
    V1TextToSpeechVoiceIdStreamInputPublish:
      oneOf:
        - $ref: '#/components/schemas/InitializeConnection'
        - $ref: '#/components/schemas/SendText'
        - $ref: '#/components/schemas/CloseConnection'

