class ConnectionAnalytic {
    constructor(pc) {
        this.pc = pc;

        this.prev = {
            video: { bytes: 0, ts: 0, frames: 0 },
            audio: { bytes: 0, ts: 0 },
            outbound: { bytes: 0, ts: 0 }
        };

        this.metrics = this.__blank();
        this.timer = null;
    }

    start() {
        if (this.timer) {
            return;
        }

        this.timer = setInterval(async () => {
            try { this._process(await this.pc.getStats()); }
            catch (e) { console.error(e); this.stop(); }
        }, 1000);
    }

    stop() {
        clearInterval(this.timer);
        this.timer = null;
    }

    __blank() {
        return {
            video: { bitrate: 0, framerate: 0, jitter: 0 },
            audio: { bitrate: 0, jitter: 0 },
            network: {
                packetLoss: 0,
                rtt: 0,
                up: 0,
                down: 0,
                agg: 0
            }
        };
    }

    async _process(stats) {
        this.metrics = this.__blank();

        for (const rep of stats.values()) {
            switch (rep.type) {
                case "inbound-rtp": this._inbound(rep); break;
                case "outbound-rtp": this._outbound(rep); break;
                case "remote-inbound-rtp": this._remote(rep); break;
                case "candidate-pair": if (rep.selected) this.__cPair(rep); break;
                case "track": this.__track(rep); break;
            }
        }

        if (!this.metrics.network.down) {
            this.metrics.network.down = this.metrics.video.bitrate + this.metrics.audio.bitrate;
        }
        if (!this.metrics.network.up && this.prev.outbound.ts) {
            // * Crude fallback
            this.metrics.network.up = this.metrics.network.down;

        }

        this.metrics.network.agg = Math.min(
            this.metrics.network.up || Infinity,
            this.metrics.network.down || Infinity
        );

        this.__ui();
    }


    __delta(prev, bytes, ts_ms) {
        const dt = (ts_ms - prev.ts) / 1000;
        const db = bytes - prev.bytes;
        return dt > 0 ? { kbps: (db * 8) / dt / 1000, dt } : { kbps: 0, dt: 1 };
    }

    _inbound(r) {
        // * browser differences
        const kind = r.kind || r.mediaType;
        if (!kind) return;

        const p = this.prev[kind];
        const { kbps, dt } = this.__delta(p, r.bytesReceived, r.timestamp);

        this.metrics[kind].bitrate = +kbps.toFixed(2);
        if (kind === "video") {
            // * Framerate: prefer framesPerSecond; fall back to framesDecoded delta
            if (r.framesPerSecond != null) {
                this.metrics.video.framerate = Math.round(r.framesPerSecond);
            } else if (r.framesDecoded != null) {
                const df = r.framesDecoded - (p.frames ?? 0);
                this.metrics.video.framerate = Math.round(df / dt);
                p.frames = r.framesDecoded;
            }
        }
        if (typeof r.jitter === "number")
            this.metrics[kind].jitter = +(r.jitter * 1000).toFixed(2);

        p.bytes = r.bytesReceived; p.ts = r.timestamp;
    }

    _outbound(r) {
        const p = this.prev.outbound;
        const { kbps } = this.__delta(p, r.bytesSent, r.timestamp);
        this.metrics.network.up = +kbps.toFixed(2);
        p.bytes = r.bytesSent; p.ts = r.timestamp;
    }

    _remote(r) {
        if (r.packetsReceived > 0) {
            this.metrics.network.packetLoss = +((r.packetsLost / r.packetsReceived) * 100).toFixed(2);
        }
        if (r.roundTripTime != null) {
            this.metrics.network.rtt = +(r.roundTripTime * 1000).toFixed(2);
        }
    }

    __cPair(r) {
        if (r.currentRoundTripTime != null)
            this.metrics.network.rtt = +(r.currentRoundTripTime * 1000).toFixed(2);
        if (r.availableOutgoingBitrate != null)
            this.metrics.network.up = +(r.availableOutgoingBitrate / 1000).toFixed(2);
        if (r.availableIncomingBitrate != null)
            this.metrics.network.down = +(r.availableIncomingBitrate / 1000).toFixed(2);
    }

    __track(r) {
        if (r.kind === "video" && r.framesPerSecond != null)
            this.metrics.video.framerate = Math.round(r.framesPerSecond);
    }


    __set(id, v) { const el = document.getElementById(id); if (el) el.textContent = v; }

    __ui() {
        this.__set("video-bitrate", this.metrics.video.bitrate.toFixed(2));
        this.__set("framerate", this.metrics.video.framerate);

        // * Prefer video jitter, fall back to audio jitter
        const jitter = this.metrics.video.jitter || this.metrics.audio.jitter;
        this.__set("jitter", jitter.toFixed(2));

        this.__set("packet-loss", this.metrics.network.packetLoss.toFixed(2));
        this.__set("rtt", this.metrics.network.rtt.toFixed(2));
        // * Show the limiting side
        this.__set("bandwidth", this.metrics.network.agg.toFixed(2));
    }
}
