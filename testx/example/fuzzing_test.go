package example

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 模糊测试：https://blog.fuzzbuzz.io/go-fuzzing-basics/

// 1. 运行go test -fuzz FuzzBasicOverwriteString命令执行测试
// 2. 模糊器在 testdata/fuzz/FuzzOverwriteString目录内存放导致问题的特定输入的文件,打开这个文件，你可以看到导致我们的函数 panic 的实际值
// 3. 运行命令重新运行测试用例检测是否修复 go test -v -count=1 -run=FuzzBasicOverwriteString/2bac7bdf139ad0b2de37275db2a606ecb335bd344500173b451e9dfc3658c12f
// 4. 重新执行第一步的命令
// 5. 一般要求模糊测试运行至少几分钟
func FuzzBasicOverwriteString(f *testing.F) {
	f.Fuzz(func(t *testing.T, str string, value rune, n int) {
		OverwriteString(str, value, n)
	})
}

// 功能性测试
func FuzzOverwriteStringSuffix(f *testing.F) {
	f.Add("Hello, world!", 'A', 8)

	f.Fuzz(func(t *testing.T, str string, value rune, n int) {
		result := OverwriteString(str, value, n)
		if n > 0 && n < utf8.RuneCountInString(str) {
			// If we modified characters [0:n], then characters [n:] should stay the same
			resultSuffix := string([]rune(result)[n:])
			strSuffix := string([]rune(str)[n:])
			if resultSuffix != strSuffix {
				t.Fatalf("OverwriteString modified too many characters! Expected %s, got %s.", strSuffix, resultSuffix)
			}
		}
	})
}

// OverwriteString 待测试的方法
// 实现的功能：对于一个字符串，用一个新的用户定义字符覆盖它的第一个字符 n 次
// 如果我们运行OverwriteString("Hello, World!", "A", 5)，正确的输出是："AAAAA, World!"。
func OverwriteString(str string, value rune, n int) string {
	// bug 1
	if n >= utf8.RuneCountInString(str) {
		return strings.Repeat(string(value), len(str))
	}

	result := []rune(str)
	// bug 2
	for i := 0; i < n; i++ {
		result[i] = value
	}
	return string(result)
}

// 最终稳定版本
func OverwriteString02(str string, value rune, n int) string {
	if n >= utf8.RuneCountInString(str) {
		return strings.Repeat(string(value), len(str))
	}

	result := []rune(str)
	for i := 0; i < n; i++ {
		result[i] = value
	}

	return string(result)
}
