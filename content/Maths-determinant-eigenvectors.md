---
title: "6. Maths4ML: Determinants & Eigenvectos"
date: "2025-12-27"
description: "Finding the stillness inside the transformation."
tags: ["Maths for ML"]
pinned: false
draft: true
---

## Matrices are opaque

As established in last blog on [matrices](./Maths-matrices.md), they are machines which transforms a vector inputed to them. But any random matrix doesn't reveal what it is doing to data. To find out the *DNA* of the matrix, these 2 tools are needed :

1. **Determinant** : tells how much space is expanding or shrinking.
2. **Eigen Vectors** : tells along which line the stretching is happening.

## Determinant (Scaling Factor)

To get the geometric intuition, let a unit square be sitting at the origin $(0,0)$. Area of the square is 1. When a matrix (linear transformation) is applied to this space, the grid line warp. The square will stretch into a long rectangle, skew into a diamond or even shrink into dot.

> Determinant is the new area of the square.

- **Det = 2** : Space is stretching. Area of square doubles.
- **Det = 0.5** : Space is contracting. Area is halved.
- **Det = 1** : Shape might change. Area remains the same.

### Singular Matrix

A matrix whose **determinant = 0**. This means the area of square becomes 0. This means the square is squashed completely flat into a single line or even a point.  

> This matrix which collapses the space is called **Singular Matrix**.

This matrix *destroys information*. When the square is squashed into a line, all information about how the original square looked like is lost. **This process is irreversible**.

> Thus, singular matrix (matrix with det = 0) is **non-invertible**.

---

For a $2 \times 2$ matrix $A$ :

$$ \begin{bmatrix} a&b \\ c&d \end{bmatrix} $$

The determinant is : 

$$ \text{det}(A) = ad - bc $$

> The determinant is the factor by which the linear transformation scales area.

## Derivation of Determinant

$$ A = \begin{bmatrix} \color{green}{a}&\color{red}{b} \\ \color{green}{c}&\color{red}{d} \end{bmatrix} $$

This transforms the basis vectors into 2 new vectors :
1. <span class = "green">Vector 1</span> : $v_1 = \color{green}{(a,c)}$
    - Moves $a$ steps right, $c$ steps up
1. <span class = "red">Vector 2</span> : $v_2 = \color{red}{(b,d)}$
    - Moves $b$ steps right, $d$ steps up

> These 2 vectors will form a parallelogram. Det is the area of this parallelogram.

