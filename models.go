package main

type AttributeChoice struct {
	id    int
	value float64
}

type DecisionData struct {
	why         string // Почему именно такое решение
	alternative string // Почему именно такое решение
	risks       string // Какие риски с ним связаны
	correction  string // Какие изменения в решении могли бы быть, если бы это была не игра а реальная жизнь
}

type Request struct {
	attributes []AttributeChoice
	hints      []int
	strategies []int
	data       DecisionData
}

type Response struct {
	// TODO
}