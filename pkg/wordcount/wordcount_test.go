package wordcount

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"spaces only", "  \n\t ", 0},
		{"plain sentence", "The quick brown fox jumps.", 5},
		{"contractions and hyphens stay one word", "Don't re-read the well\u2011known book", 5},
		{"dashes between words split them", "this\u2014that and here\u2013there", 5},
		{"punctuation alone is no word", "Wait \u2026 ! \u2014 ?", 1},
		{"numbers are words", "Chapter 12 of 3.5", 4},
		{"accented letters", "Ünïcödé café naïve", 3},
		{"zero-width space separates", "one\u200btwo", 2},
		{"each CJK character is a word", "日本語を読む", 6},
		{"katakana with a long vowel mark", "コーヒー", 4},
		{"CJK punctuation is no word", "「本」。", 1},
		{"CJK next to latin", "Shisho本棚", 3},
		{"Hangul is spaced like latin", "책을 읽다", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Text(tt.text))
		})
	}
}

func TestHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want int
	}{
		{
			"counts the body, not the head",
			`<html><head><title>Chapter One Title</title></head><body><h1>Chapter 1</h1><p>This is a test chapter.</p></body></html>`,
			7,
		},
		{
			"inline markup does not split a word",
			`<p>Hel<b>lo</b> <span class="koboSpan">wor</span><i>ld</i></p>`,
			2,
		},
		{
			"block markup splits words",
			`<p>one</p><p>two</p><div>three<br/>four</div><li>five</li>`,
			5,
		},
		{
			"unknown elements split words",
			`<p>Mobipocket marks<mbp:nu>words</mbp:nu>too</p>`,
			4,
		},
		{
			"scripts, styles, and ruby annotations are skipped",
			`<style>p { color: red }</style><script>var x = "a b c";</script><p><ruby>漢字<rp>(</rp><rt>かんじ</rt><rp>)</rp></ruby></p>`,
			2,
		},
		{
			"a self-closed script, as XHTML allows, ends where it opens",
			`<html><head><script type="text/javascript" src="kobo.js"/><title>T</title></head><body><p>one two</p></body></html>`,
			2,
		},
		{
			"other self-closed elements whose content isn't markup",
			`<p>one</p><style/><title/><textarea/><p>two</p><p>three</p>`,
			3,
		},
		{
			"CDATA is text, even holding a >",
			`<p><![CDATA[one > two]]> three</p><p>four</p>`,
			4,
		},
		{
			"entities decode",
			`<p>Tom&nbsp;&amp;&nbsp;Jerry&#8212;friends</p>`,
			3,
		},
		{
			"self-closing XHTML",
			`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><body><p>a<br/>b</p><img src="x.png"/><p>c</p></body></html>`,
			3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := HTML(strings.NewReader(tt.html))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHTML_Encodings(t *testing.T) {
	t.Parallel()

	doc := `<html><head><meta charset="windows-1252"/></head><body><p>日本語 café</p></body></html>`

	t.Run("UTF-8 without a byte order mark, whatever the meta says", func(t *testing.T) {
		t.Parallel()
		got, err := HTML(strings.NewReader(doc))
		require.NoError(t, err)
		assert.Equal(t, 4, got)
	})

	t.Run("UTF-16 with a byte order mark", func(t *testing.T) {
		t.Parallel()
		units := utf16.Encode([]rune(doc))
		b := []byte{0xff, 0xfe}
		for _, u := range units {
			b = append(b, byte(u), byte(u>>8))
		}
		got, err := HTML(bytes.NewReader(b))
		require.NoError(t, err)
		assert.Equal(t, 4, got)
	})
}
