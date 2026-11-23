/* Gunship 2000 audio-preparation helper, 2026.
 * SPDX-License-Identifier: GPL-2.0-or-later
 * Uses the separately retained DOSBox DBOPL synthesis core and its notices.
 * This program converts an ordered register timeline; it is not a runtime player.
 */
#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <iostream>
#include <stdexcept>
#include <vector>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif
#include "dbopl.h"

namespace DBOPL { void InitTables(); }

static uint64_t read_le(unsigned count) {
    uint64_t value = 0;
    for (unsigned i = 0; i < count; ++i) {
        const int c = std::cin.get();
        if (c == EOF) throw std::runtime_error("truncated register timeline");
        value |= uint64_t(uint8_t(c)) << (8 * i);
    }
    return value;
}
static void write32(int32_t value) {
    const auto u = uint32_t(value);
    for (unsigned i = 0; i < 4; ++i) std::cout.put(char((u >> (8 * i)) & 255));
}
int main(int argc, char** argv) {
    try {
#ifdef _WIN32
        // The binary protocol must never undergo Windows CR/LF translation.
        _setmode(_fileno(stdin), _O_BINARY);
        _setmode(_fileno(stdout), _O_BINARY);
#endif
        // The flight adapter keeps a single Chip across bounded transactions.
        // Preparation retains the original OPL27 protocol byte-for-byte.
        if (argc == 2 && std::string(argv[1]) == "--stream") {
            char magic[8]{}; std::cin.read(magic, 8);
            if (std::string(magic, 8) != std::string("OPL32\0\0\0", 8)) throw std::runtime_error("invalid stream signature");
            const unsigned rate = unsigned(read_le(4));
            if (rate < 8000 || rate > 192000) throw std::runtime_error("invalid stream rate");
            DBOPL::InitTables(); DBOPL::Chip chip(true); chip.Setup(rate);
            std::vector<int32_t> buffer(512);
            while (std::cin.peek() != EOF) {
                const unsigned frames = unsigned(read_le(4)), count = unsigned(read_le(4));
                if (!frames || frames > 8192 || count > 8192) throw std::runtime_error("stream transaction exceeds bounds");
                unsigned at = 0;
                const auto generate = [&](unsigned end) {
                    if (end < at || end > frames) throw std::runtime_error("invalid stream timestamp");
                    while (at < end) {
                        const unsigned n = std::min<unsigned>(end-at, buffer.size());
                        chip.GenerateBlock2(n, buffer.data());
                        for (unsigned j = 0; j < n; ++j) write32(buffer[j]);
                        at += n;
                    }
                };
                for (unsigned i=0; i<count; ++i) {
                    const unsigned frame = unsigned(read_le(4)), reg = unsigned(read_le(2));
                    const unsigned value = unsigned(read_le(1)), reserved = unsigned(read_le(1));
                    if (reg > 255 || reserved) throw std::runtime_error("invalid stream register");
                    generate(frame); chip.WriteReg(reg, uint8_t(value));
                }
                generate(frames); std::cout.flush();
                if (!std::cout) throw std::runtime_error("stream output failed");
            }
            return 0;
        }
        if (argc != 1) throw std::runtime_error("read OPL27 timeline on stdin; optional --stream retains chip state");
        char magic[8]{}; std::cin.read(magic, 8);
        if (std::string(magic, 8) != std::string("OPL27\0\0\0", 8)) throw std::runtime_error("invalid timeline signature");
        const unsigned rate = unsigned(read_le(4));
        const unsigned count = unsigned(read_le(4));
        const uint64_t frames = read_le(8);
        const unsigned opl3capable = unsigned(read_le(4));
        if (rate < 8000 || rate > 192000 || count > 5000000 || frames > uint64_t(rate) * 600 || opl3capable > 1)
            throw std::runtime_error("timeline exceeds preparation limits");
        DBOPL::InitTables();
        DBOPL::Chip chip(opl3capable != 0);
        chip.Setup(rate);
        constexpr unsigned block = 512;
        std::vector<int32_t> buffer(block);
        uint64_t at = 0;
        const auto generate_to = [&](uint64_t end) {
            if (end < at || end > frames) throw std::runtime_error("nonmonotonic or out-of-range timestamp");
            while (at < end) {
                const unsigned n = unsigned(std::min<uint64_t>(end - at, block));
                chip.GenerateBlock2(n, buffer.data());
                for (unsigned j = 0; j < n; ++j) write32(buffer[j]);
                at += n;
            }
        };
        for (unsigned i = 0; i < count; ++i) {
            const uint64_t frame = read_le(8);
            const unsigned reg = unsigned(read_le(2));
            const unsigned value = unsigned(read_le(1));
            const unsigned reserved = unsigned(read_le(1));
            if (reg > 255 || reserved) throw std::runtime_error("only the selected mono OPL register path is supported");
            generate_to(frame);
            chip.WriteReg(reg, uint8_t(value));
        }
        if (std::cin.get() != EOF) throw std::runtime_error("trailing timeline bytes");
        generate_to(frames);
        std::cout.flush();
        if (!std::cout) throw std::runtime_error("PCM output failed");
        return 0;
    } catch (const std::exception& error) {
        std::cerr << "audio preparation: " << error.what() << '\n';
        return 1;
    }
}
