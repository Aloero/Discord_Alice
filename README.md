
---

# Discord Music Bot

A Discord bot that allows users to play music in voice channels, manage playback, and interact via slash commands.

## Features

- Join voice channels and play audio from various sources.
- Support for playback control commands: play, pause, skip, rewind, and leave.
- Voice activity detection to manage playback.
- Slash command integration for user-friendly interaction.

## Requirements

- Go 1.18 or higher
- DiscordGo package
- Opus codec library
- WebRTC VAD (Voice Activity Detection) library
- gRPC for communication (if applicable)

## Installation

1. **Clone the repository:**
   ```bash
   git clone <repository-url>
   cd <repository-directory>
   ```

2. **Install Go dependencies:**
   ```bash
   go mod tidy
   ```

3. **Set up your Discord bot:**
   - Create a bot on the [Discord Developer Portal](https://discord.com/developers/applications).
   - Copy your bot's Token, Application ID, Public Key, Client Secret, and set them in the constants section of `main.go`.

4. **Run the bot:**
   ```bash
   go run main.go
   ```

## Configuration

Update the following constants in `main.go`:

```go
const (
    applicationID = "<Your Application ID>"
    publicKey     = "<Your Public Key>"
    clientSecret  = "<Your Client Secret>"
    guildID       = "<Your Guild ID>"
    channelID     = "<Your Channel ID>"
    token         = "<Your Bot Token>"
    // Other constants...
)
```

## Commands

- **/start**: Join the voice channel.
- **/play [query-or-link]**: Request to play music.
- **/leave**: Leave the voice channel.
- **/clear-queue**: Clears the bot's audio queue.
- **/skip**: Skips the current track.
- **/pause**: Pauses playback.
- **/continue**: Resumes playback.
- **/rewind [seconds]**: Rewinds playback by the specified number of seconds.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any changes or enhancements.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [DiscordGo](https://github.com/bwmarrin/discordgo) - The Discord API library for Go.
- [Opus](https://github.com/hraban/opus) - Opus codec for audio encoding and decoding.
- [WebRTC VAD](https://github.com/maxhawkins/go-webrtcvad) - Voice Activity Detection for managing audio streams.

---
