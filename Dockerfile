FROM golang:1.25-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/server ./cmd/server

# Font for ffmpeg's drawtext filter, which burns the watermark into
# recordings — the distroless runtime below has no fontconfig to discover
# system fonts, so one is bundled explicitly instead.
RUN apt-get update && apt-get install -y --no-install-recommends fonts-dejavu-core \
	&& rm -rf /var/lib/apt/lists/*

# Statically-linked ffmpeg build, since distroless/static has no dynamic
# linker for a normal distro package to run against.
# TODO: pin this to a specific version tag once one's been verified good,
# instead of floating on :latest.
FROM mwader/static-ffmpeg:latest AS ffmpeg

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/server /server
COPY --from=builder /usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf /fonts/DejaVuSans-Bold.ttf
COPY --from=ffmpeg /ffmpeg /usr/local/bin/ffmpeg
ENV FFMPEG_PATH=/usr/local/bin/ffmpeg
ENV WATERMARK_FONT_PATH=/fonts/DejaVuSans-Bold.ttf
EXPOSE 8080
ENTRYPOINT ["/server"]
