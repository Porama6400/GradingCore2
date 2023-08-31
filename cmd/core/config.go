package main

type Config struct {
	templates map[string]Template
}

type Template struct {
	id    string
	image string
}
