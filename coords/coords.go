package coords

import (
	"errors"
	"math"
)

type Matrix [6]float64
func Identity() Matrix { return Matrix{1,0,0,1,0,0} }
func (m Matrix) Multiply(o Matrix) Matrix { return Matrix{ m[0]*o[0]+m[1]*o[2], m[0]*o[1]+m[1]*o[3], m[2]*o[0]+m[3]*o[2], m[2]*o[1]+m[3]*o[3], m[4]*o[0]+m[5]*o[2]+o[4], m[4]*o[1]+m[5]*o[3]+o[5] } }

type Point struct{ X,Y float64 }
func (m Matrix) Transform(p Point) Point { return Point{ X: m[0]*p.X + m[2]*p.Y + m[4], Y: m[1]*p.X + m[3]*p.Y + m[5] } }
func (m Matrix) Inverse() (Matrix, error) { det := m[0]*m[3] - m[1]*m[2]; if math.Abs(det) < 1e-10 { return Matrix{}, errors.New("matrix singular") }; return Matrix{ m[3]/det, -m[1]/det, -m[2]/det, m[0]/det, (m[2]*m[5]-m[3]*m[4])/det, (m[1]*m[4]-m[0]*m[5])/det }, nil }
func Translate(tx, ty float64) Matrix { return Matrix{1,0,0,1,tx,ty} }
func Scale(sx, sy float64) Matrix { return Matrix{sx,0,0,sy,0,0} }
func Rotate(angle float64) Matrix { c:=math.Cos(angle); s:=math.Sin(angle); return Matrix{c,s,-s,c,0,0} }
