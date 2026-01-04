# Quick Start Guide

Get your simultaneous interpretation system running in **5 minutes**.

## Prerequisites

- ✅ Google API Key ([Get one here](https://makersuite.google.com/app/apikey))
- ✅ Go 1.21+ installed
- ✅ Chrome or Firefox browser
- ✅ Microphone + Headphones

## 3-Step Setup

### Step 1: Configure (1 minute)

```bash
cd examples/simultaneous-interpretation-gemini

# Create .env file
cp .env.example .env

# Edit .env and add your API key
nano .env
# Add: GOOGLE_API_KEY=your-key-here
```

### Step 2: Install (2 minutes)

```bash
# Install dependencies
go mod download
```

### Step 3: Run (30 seconds)

```bash
# Start the server
go run main.go

# You should see:
# ╔═══════════════════════════════════════════════════╗
# ║  Real-time Simultaneous Interpretation (Gemini)  ║
# ╚═══════════════════════════════════════════════════╝
#
# 📋 Configuration:
#    Model: gemini-2.5-flash-native-audio-preview-12-2025
#    Source: Chinese → Target: English
#    Domain: casual
#
# ✅ Server ready!
# 🌐 Open http://localhost:8080 in your browser
```

### Step 4: Use (30 seconds)

1. Open http://localhost:8080
2. Click "Connect"
3. Allow microphone access
4. **Put on headphones** (prevents echo!)
5. Start speaking in your source language
6. Hear the interpretation in your target language!

**Total time: ~4 minutes** ⏱️

## Configuration Examples

### Chinese → English (Business)
```env
SOURCE_LANG=Chinese
TARGET_LANG=English
INTERPRETATION_DOMAIN=business
```

### English → Spanish (Casual)
```env
SOURCE_LANG=English
TARGET_LANG=Spanish
INTERPRETATION_DOMAIN=casual
```

### Japanese → English (Technical)
```env
SOURCE_LANG=Japanese
TARGET_LANG=English
INTERPRETATION_DOMAIN=technical
```

## Troubleshooting

### "GOOGLE_API_KEY environment variable is required"
→ Make sure .env file exists and contains `GOOGLE_API_KEY=your-key`

### "Connection failed"
→ Check your API key is valid and has credit

### Can't hear audio
→ **Use headphones!** Speaker output causes echo

### High latency
→ Check your internet connection

## Next Steps

- 📖 Read [README.md](README.md) for full documentation
- 📊 See [COMPARISON.md](COMPARISON.md) for performance details
- ⚙️ Customize `.env` for your use case
- 🎨 Explore different domains and language pairs

## Support

- GitHub Issues: [realtime-ai/realtime-ai](https://github.com/realtime-ai/realtime-ai/issues)
- Documentation: See main repository

---

**Enjoy real-time interpretation! 🚀**
