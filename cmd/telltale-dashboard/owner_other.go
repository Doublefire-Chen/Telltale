//go:build !unix

package main

func checkDataDirOwner(string) error { return nil }
