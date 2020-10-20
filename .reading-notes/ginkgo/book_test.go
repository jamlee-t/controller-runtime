package ginkgo_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/example/books"
	. "github.com/onsi/gomega"
)

var _ = Describe("Book", func() {
	var (
		longBook  Book
		shortBook Book
	)

	BeforeEach(func() {
		GinkgoWriter.Write([]byte("hello, before each\n"))
		longBook = Book{
			Title:  "Les Miserables",
			Author: "Victor Hugo",
			Pages:  1488,
		}

		shortBook = Book{
			Title:  "Fox In Socks",
			Author: "Dr. Seuss",
			Pages:  24,
		}
	})
	JustBeforeEach(func() {
		GinkgoWriter.Write([]byte("hello, just before each\n"))
	})

	Describe("Categorizing book length", func() {
		GinkgoWriter.Write([]byte("归类一\n\n\n"))

		Context("With more than 300 pages", func() {
			GinkgoWriter.Write([]byte("归类一/子归类一\n\n\n"))
			It("should be a novel", func() {
				GinkgoWriter.Write([]byte("归类一/子归类一/用例一\n\n\n"))
				Expect(longBook.CategoryByLength()).To(Equal("NOVEL"))
			})
		})

		Context("With fewer than 300 pages", func() {
			GinkgoWriter.Write([]byte("归类一/子归类二\n\n\n"))
			BeforeEach(func() {
				GinkgoWriter.Write([]byte("hello, context before each\n"))
			})
			It("should be a short story", func() {
				GinkgoWriter.Write([]byte("归类一/子归类二/用例一\n\n\n"))
				Expect(shortBook.CategoryByLength()).To(Equal("SHORT STORY"))
			})
			It("should be a short story", func() {
				GinkgoWriter.Write([]byte("归类一/子归类二/用例二\n\n\n"))
				Expect(shortBook.CategoryByLength()).To(Equal("SHORT STORY"))
			})
		})
	})
})
