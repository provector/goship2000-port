// Preparation type shim, 2026-10-23. SPDX-License-Identifier: GPL-2.0-or-later
#pragma once
#include <cstdint>
#include <cstddef>
#include <cstdio>
using Bitu = uintptr_t;
using Bits = intptr_t;
#define DB_FASTCALL
#if defined(__GNUC__)
#define GCC_UNLIKELY(x) __builtin_expect((x),0)
#else
#define GCC_UNLIKELY(x) (x)
#endif
#define LOG_MSG(...) std::fprintf(stderr, __VA_ARGS__)
