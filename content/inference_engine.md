---
title: "Inference Engine"
date: "2025-11-24"
description: "Building an infernece engine for machine translation transformer model in C++."
tags: ["Transformers"]
pinned: true
---

# Writing an Inference Engine for transformer from scratch

This is kind of a work-log / explanation of the [**pure C++ Transformer Inference Engine**](https://github.com/Kush-Singh-26/Inference_Engine). I made this to use and learn more about C++ other than just DSA stuff. 

It is made specifically for a basic [encoder-decoder transformer for english-hindi machine translation](https://huggingface.co/Kush26/Transformer_Translation) task. This engine doesn't depend on any external dependecies or frameworks like PyTorch, TensorFlow, or ONNX Runtime.

## Graph-Less Approach / Eager Execution

This project operates operates imperatively (immdeiate mode) similar to `llama.cpp` or `GGML`. Deep learning frameworks build a computation graph, which represents the flow of data, allowing the framework to manage memory allocation, automatic differentiation, etc. But for infernece purposes, there are no need for these tasks.

When an operation like : `C = A * B` is called, graph based system often just adds a *Multiplication Node* to a graph to be run later. But here, the actual math loops are executed the moment it is called.

# Core parts of the engine

## MatMul

Matrix Multiplication or **MatMul** is the most basic thing in a neural network. I have explored from a naive implementation to high-performance kernel using `AVX2` intrinsics and quantization.

### 1. Naive Implementation

- It is the most basic matmul implementation.

```cpp
static Matrix matmul(const Matrix& A, const Matrix& B){
            assert(A.cols == B.rows);

            Matrix C(A.rows, B.cols);

            for(size_t i = 0;i< A.rows; i++){
                for(size_t j = 0; j< B.cols;j++){
                    float sum = 0.0f;
                    for(size_t k = 0;k<A.cols;k++)
                        sum += A(i,k) * B(k,j);
                    C(i,j) = sum;
                }
            }

            return C;
        }
```

- It computes the final value of a index in the resultant matrix before going to next index.

>- Problem :
>   - Elements in Matrix B are accessed like : `B[0][0]`, `B[1][0]`, `B[2][0]`.
>   - This lead to **cache-miss** as the cache loads the elements `B[0][1]`, `B[0][2]`, ... when `B[0][0]` is called.

- For 2 Matrices of size $1024 \times 1024$, *time taken = 10.3328 seconds*

### 2. Loop Reordering

- To address the problem of cache miss in above implementation, loops are reordered so that inner loop iterates over columns of B (`j`) instead of the common dimension (`k`).

```cpp
static Matrix matmul(const Matrix& A, const Matrix& B){
            assert(A.cols == B.rows);

            Matrix C(A.rows, B.cols);

            for(size_t i = 0;i< A.rows; i++){
                for(size_t k = 0; k< A.cols;k++){
                    float valA = A(i,k);

                    for(size_t j = 0;j<B.cols;j++)
                        C(i,j) += valA * B(k, j);
                }
            }

            return C;
```

<img src="/static/images/Infer1.png" alt="Image" width="800" height="300">

- This improved `ikj` loop instead of the old `ijk` loop loads `A[i,k]` once and then access an entire row of Matrix B via `B[k][j]` from RAM.
- So instead of completely computing a value by constantly jumping between rows, this loop reads the complete row of B and then goes to next row and accumulating the final value.

- For 2 Matrices of size $1024 \times 1024$, *time taken = 7.78992  seconds*

<a id="parallelization-openmp"></a>
### 3. Parallelization (OpenMP) 

- Its time to parallize the code.

```cpp
static Matrix matmul(const Matrix& A, const Matrix& B){
            assert(A.cols == B.rows);

            Matrix C(A.rows, B.cols);
            
            #pragma omp parallel for
            for(size_t i = 0;i< A.rows; i++){
                for(size_t k = 0; k< A.cols;k++){
                    float valA = A(i,k);

                    for(size_t j = 0;j<B.cols;j++)
                        C(i,j) += valA * B(k, j);
                }
            }

            return C;
        }
```

- Use `OpenMP` (Open Multi Processing) API to parallelize the loop on multi-core CPUs.
- The only addition is `#pragma omp parallel for` just before the loop.
    - It tells the OpenMP compiler and runtime to create a team of threads and divide the loop’s iterations among them. 
    - Since each iteration corresponds to processing one row of matrix A, the work is split across multiple threads, which the runtime schedules on available CPU cores. 
    - This results in concurrent, multi-core execution of the matrix multiplication.

- For 2 Matrices of size $1024 \times 1024$, *time taken = 2.3668 seconds  seconds*

> A `pragma` is a special instruction you give to the compiler to request optional behavior that is not part of the core C/C++ language.

---

Before, going to the final optimization, lets address how the matrices are stored and how the engine works!

## Matrix Organization

- The engine uses a Flat Memory Layout, instead of a *jagged array* (`float*** array`), in a Row-Major-Order.
- Because all data is in one continuous block, the CPU can pre-fetch data efficiently. 
- Iterating through the vector sequentially `(data[i], data[i+1])` is significantly faster than chasing pointers.

- RAII (Resource Acquisition Is Initialization) is followed in C++.
    - When a `Tensor` is created, it acquires resources. 
    - When it goes out of scope(e.g., when a function ends), the C++ runtime automatically destroys it.

> This prevents Memory Leaks.

Because the memory is 1D but the logic is N-dimensional, engine uses `Strides` to translate coordinates.

> `Strides` : It is a vector constructed during construction. It represents how many elements must be skipped in the flat array to move one step in a specific dimension.

- `Strides` are calculated backwards.
    - Last dimension will have stride of 1.
    - Second-last dimension will have stride of **equal to size of last dimension**.
    - And so on.

- `stride[i]` = product of all the dimension from `i+1` to `n`.
- If dimensions are : $D_0, D_1, \cdots, D_{n-1}$, then :

$$ \text{stride[i]} = D_{i+1} \times D_{i+2} \times \cdots \times D_{n-1} $$

- To access elements at a coordinate, engine calculates the flat index uisng the *Dot Product* of coordinates and strides.

- Thus for coordinates : `[b, r, c]` :

$$ \text{Flat Index} = (b \times stride_0) + (r \times stride_1) \times (c \times stride_2) $$

- Implemented in `get_index` in `tensor.cpp`.

```cpp
size_t Tensor::get_index(const std::vector<size_t>& indices) const {
    assert(indices.size() == shape.size());
    size_t index = 0;
    for (size_t i = 0; i < indices.size(); i++) {
        index += indices[i] * strides[i];
    }
    return index;
}
```

---

Now that how matrices are stored and accessed is understood, lets see how the Engine is organized.

## The Structure of the engine

```txt
ml_engine
├── Makefile
├── include
│   ├── model.h
│   ├── tensor.h
│   └── tokenizer.h
├── main.cpp
└── src
    ├── model.cpp
    └── tensor.cpp
```

- `tensor.h` & `tensor.cpp` : It contains the core tensor/matrix operations.
- `model.h` & `model.cpp` : It defines all the components of the Transformer model.
- `tokenizer.h` : It is used to use the tokenizer to tokenize the input text.
- `main.cpp` : It is controller, which calls all the files modules and loads the tokenizer and model weights and perform infernece.

---

# Tensor Class

It is the core class for managing multi-dimensional arrays of 32-bit floating-point numbers `(float)`. It handles memory management, shape metadata, and high-performance mathematical operations. It is defined in the files `tensor.h` & `tensor.cpp`.

#### Members 

- `data` : A flat `std::vector<float>` containing all elements.
- `shape` : A vector defining dimensions (eg. `{row, col}`).
- `strides` : As discussed above, pre-calculated step sizes for navigating the flat `data` vector.

### Constructors 

The **Rule of 5** is applied here. It means that if a class wns resources and one of the constructor (function) is written, then there is a need to write the other four too, to avoid bugs like double frees, memory leaks, or unintended copying.

1. Destructor
2. Copy constructor
3. Copy assignment operator
4. Move constructor
5. Move assignment operator

> Destructor is implicitly applied as RAII handles the cleanup.

Out of these the 4 types of constructors are discussed below:

#### 1. Default Constructor

```cpp
Tensor(){}
```

- Creates an empty tensor with no shape and no allocated memory.

#### 2. Parameterized Constructor

```cpp
Tensor::Tensor(std::vector<size_t> s) : shape(s) {
    size_t total_size = 1;
    strides.resize(shape.size());
    size_t stride = 1;
    for (int i = shape.size() - 1; i >= 0; i--) {
        strides[i] = stride;
        total_size *= shape[i];
        stride *= shape[i];
    }
    data.resize(total_size); 
}
```

- It is the primary way to create a tensor. 
- It initializes the shape with vector `s`, calculates the necessary `strides` for navigation, and allocates the required memory in `data` (initialized to zero).

#### 3. Copy Constructor

```cpp
Tensor::Tensor(const Tensor& other) 
    : data(other.data), shape(other.shape), strides(other.strides) {}
```

- Creates a deep copy (clone) of another tensor. 
- It allocates new memory and duplicates every element from other to the new object.
- Slow.

#### 4. Move Constructor / steals

```cpp
Tensor::Tensor(Tensor&& other) noexcept 
    : data(std::move(other.data)), shape(std::move(other.shape)), strides(std::move(other.strides)) {}
```

- Example : `Tensor B = std::move(A);`
- It **steals** the internal resources of another object rather than copying them.
- `Tensor&&` : It means this is an **R-value reference**.
    - It tells the compiler that the object coming in is temporary, so it is about to be destroyed.
    - Thus it is safe to loot.
- `std::move(other.data)` :
    - `other.data` : It is a `std::vector`
        - `std::move(...)` does not copy its memory.
        - It enables the vector’s move constructor, which :
    - Copies the internal pointer
    - Copies the size & capacity
    - Sets the old vector’s pointer to nullptr (or empty state)
- `noexcept` promises that this constructor cannot throw exceptions.
    - This enables stadard container to choose move instead of copy and enabling massive performance gains.

---

> ## Some new C++ Concepts used till now
> - These are new for me as I am just learning. 
> ### Memeber Initializer list (`:`)
> - it is the most efficient way to initialize variables inside a class.
> - eg. In parameterized constructor, tt tells the compiler: 
>   - Construct the variable shape directly using the value s.
> - If `shape = s` was used inside `{}`, then it will lead to calling Default Constructor + 1 Assignment Operator + 1 Destructor
>
> ### Scope Resolution Operator (`::`)
> - It tells the compiler which folder to look inside to find a specific tool.
> - It is like the `/` is file path.
>   - `Tensor::Tensor` : Look inside the Tensor class (folder) to find the Tensor function.
>   - `std::vector` : Look inside the std library (folder) to find the vector tool.
>
> ### Operator Overloading (`[]`)
>To allow accessing the elements like : `float value = A[{2, 5}]`, instead of `float value = A.data[A.get_index({2, 5})]`
>```cpp
>float& Tensor::operator[](const std::vector<size_t>& indices) {
>    return data[get_index(indices)];
>}
>```
> `operator` function allows to overload the built-in operators like (`[]`, `+`, `*`, etc.) to work with custom classes.

---

Now comes the most interesting part of the engine, the final mal-mul implementation and the hardware accleration used to implement it.

## SIMD (Single Instruction, Multiple Data) / Intrinsic Functions

Normally when 2 arrays are added, CPU loads one number, adds it and stores it. This is repeated for every element. This is known as **SISD** (Single Instruction, Single Data), i.e., a single processor executes one instruction at a time on one data stream. These are all *Scalar* operations.

To improve the performance, **SIMD** operations are done. This allows the CPU to load a pack of numbers (*vector*) and perform the same operation (*instruction*) on all of them simulaneously in a single clock cycle.

These SIMD operations are performed using `AVX / AVX2` (Advanced Vector Extensions) insruction sets. `AVX` introduced **256-bit registers**, called `YMM` registers.
- A `YMM` regsiter is 256 bit and a standard `float` is 32 bits.
- Thus $\frac{256}{32} = 8$ floats can be processed in a single operations.

- `<immintrin.h>` is the header that is used to access access to the CPU's specific instruction sets (like AVX, SSE, BMI) without writing raw Assembly code.

### SIMD Intrinsic Naming Scheme 

**General format:**

`_mm[width]_[operation][inputType]_[outputType]`

#### Vector Width
- `_mm_` → 128-bit (SSE)
- `_mm256_` → 256-bit (AVX/AVX2)

#### Data Types
- `ps` → packed single-precision floats
- `pd` → packed double-precision floats
- `ss` / `sd` → scalar float/double
- `epi8` / `epi16` / `epi32` / `epi64` → packed signed integers

#### Common Operations
- `add`, `sub`, `mul`, `max`, `min` → arithmetic
- `fmadd`, `fnmadd` → fused multiply-add
- `cvt` → convert between types
- `cast` → reinterpret bits without converting
- `loadu` / `storeu` → unaligned memory ops
- `set1` / `setzero` → broadcast or zero fill
- `hadd` → horizontal add
- `extractf128` → extract 128-bit lane

#### Interpretation Examples
- `_mm256_fmadd_ps` → 256-bit fused multiply-add on floats  
- `_mm256_cvtepi8_epi32` → widen int8 → int32  
- `_mm256_castsi256_ps` → reinterpret int bits as floats  
- `_mm_hadd_ps` → horizontal add (SSE)

---

The table below contains the list of all intrinsic functions used in the `tensor.cpp`.

| Intrinsic Function         | Description                                                                  | Purpose in `tensor.cpp`                                                                                      |
|----------------------------|------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| `_mm256_fmadd_ps`          | **Fused Multiply-Add**. Calculates `(a×b)+c` in one step.                          | Core of the matrix multiplication kernel (`matmul_2d`, `matmul_q8`) for fast dot products.                      |
| `_mm256_fnmadd_ps`         | **Fused Negative Multiply-Add**. Calculates `−(a×b)+c`.                            | Used in `fast_exp_avx2` for range reduction (calculating the remainder).                                     |
| `_mm256_add_ps`            | Adds two float vectors element-wise.                                         | Used in softmax (summing exponentials) and `fast_exp_avx2` (polynomial terms).                                 |
| `_mm256_sub_ps`            | Subtracts vector B from A element-wise.                                      | Used in softmax to subtract the max value `(x−xmax)` for numerical stability.                                  |
| `_mm256_mul_ps`            | Multiplies two float vectors element-wise.                                   | Used in `fast_exp_avx2` for polynomial approximation and final result scaling.                                 |
| `_mm256_max_ps`            | Returns the maximum of two vectors element-wise.                             | Used in softmax to clamp extremely negative numbers to avoid underflow artifacts.                            |
| `_mm256_round_ps`          | Rounds floating-point values to nearest integer (returns float).             | Used in `fast_exp_avx2` to calculate k = round($x \cdot log_2 e$).                                                    |
| `_mm256_set1_ps`           | Broadcasts a single float to all 8 elements of a vector.                     | Replicates scalars (like val_A or scale) across the vector for broadcasting.                                 |
| `_mm256_setzero_ps`        | Creates a vector containing all zeros.                                       | Initializes the accumulator (vec_acc) before matrix multiplication loops.                                    |
| `_mm256_loadu_ps`          | Loads 256 bits (8 floats) from unaligned memory.                             | Loads weights from Tensor B during matrix multiplication.                                                    |
| `_mm256_storeu_ps`         | Stores 256 bits (8 floats) into unaligned memory.                            | Writes computed results to the output tensor buffer.                                                         |
| `_mm_loadl_epi64`          | Loads the lower 64 bits (8 bytes) into a 128-bit integer register.           | Critical for `matmul_q8`: Loads 8 Int8 quantized weights at once to save bandwidth.                           |
| `_mm256_cvtepi8_epi32`     | Sign-extends 8-bit integers to 32-bit integers.                              | Expands packed Int8 weights so they can be converted to floats.                                              |
| `_mm256_cvtepi32_ps`       | Converts 32-bit integers to 32-bit floating-point numbers.                    | Final step in de-quantizing weights in `matmul_q8`.                                                            |
| `_mm256_cvtps_epi32`       | Converts floats to 32-bit integers (truncation).                             | Used in `fast_exp_avx2` to convert exponent k into an integer for bitwise shifting.                            |
| `_mm256_castsi256_ps`      | Reinterpret cast: Treats integer bits as float bits (no conversion).         | Used in `fast_exp_avx2` to write the exponent directly into float bit representation.                          |
| `_mm256_castps256_ps128`   | Extracts lower 128 bits (4 floats) from a 256-bit vector.                    | Used in `hsum_avx2` to start horizontal sum.                                                                   |
| `_mm256_extractf128_ps`    | Extracts upper 128 bits from a 256-bit vector.                               | Used in `hsum_avx2` to fold upper half onto lower half.                                                        |
| `_mm256_add_epi32`         | Adds 32-bit integers element-wise.                                           | Adds bias (127) to exponent in `fast_exp_avx2`.                                                                |
| `_mm256_slli_epi32`        | Shift-left logical by immediate.                                             | Shifts exponent 23 bits left to align with IEEE-754 exponent field.                                          |
| `_mm256_set1_epi32`        | Broadcasts a 32-bit integer to all elements.                                 | Used to create bias vector (127) for exponent manipulation.                                                   |
| `_mm_add_ps`               | Adds two 128-bit (4-float) vectors.                                          | Adds the low and high halves of a 256-bit vector.                                                            |
| `_mm_hadd_ps`              | Horizontal add: `[a+b, c+d]`.                                                  | Folds vector to compute final sum.                                                                           |
| `_mm_cvtss_f32`            | Moves lowest float from register to a C++ variable.                          | Extracts final horizontal sum into a standard float.                                                         |

---

# MatMul

## `matmul_2d`

Now lets see how `matmul_2d` works. It is the core of the `float` x `float` multiplications.

- It wiil be used in 2 parts :
    1. $Q \times K^T$ : `Tensor Scores = Q_h * K_h_T;` (in `model.cpp`)
    2. $\text{Scores} \times V$ : `Tensor Out_h = Scores * V_h;`

> I only will discuss small snipets from the code.

- The function accepts 2 `Tensor` : `const Tensor& A, const Tensor& B`.

```cpp
size_t K = A.shape.back(); // returns the last dimension of A
if (B.shape.size() != 2 || B.shape[0] != K) { // ensure the last and first dim of A and B match & B is 2D
    std::cerr << "MatMul Error: Shape mismatch\n";
    exit(1);
}
size_t M = B.shape[1]; // no of cols in B and last dim of C
size_t total_rows_A = 1;
for (size_t i = 0; i < A.shape.size() - 1; i++) total_rows_A *= A.shape[i];

std::vector<size_t> C_shape = A.shape;
C_shape.back() = M;
Tensor C(C_shape);
```

- `A` can be multi dimensional and `B` must be 2D.
- `total_rows_A` is calculated by *flattening* all dim of `A`, except the last one.

- The resultant matrix `C` will have same dims as `A`, except the last dim, which will be `K` same as last dim of `B`.

> Thus, `A[..., K]  ×  B[K, M]  →  C[..., M]`

Now, that `C` is defined, it is time to start the actual **matmul**. In the last optimization [above](#parallelization-openmp), we used *row optimization*, i.e., split the rows among the tokens / rows. But the actual matmul in transformer can have 2 cases :

- `total_rows_A>1` : when the prompt consisting of multiple tokens is being processed in the encoder part
   - `Q_h * K_h_T` : Shape of A : `[Prompt_Len, Head_Dim]` & shape of B : `Head_Dim, Prompt_Len`.
   - `Scores * V` : Shape of A : `[Prompt_Len, Prompt_Len]` & shape of B : `[Prompt_Len, Head_Dim]`.
   - In this case, it is good to stick with parallelizing the rows.

- `total_rows_a == 1` : When *only one token is being processed* in decoder during auto-regressive generation.
    - Same as above places, but just in 2 places : Self and Cross Attention.
    - Shape of A is `[1, Head_Dim]` or `[1, Context_len]`.
    - In this case, use **Column Parallelism**.
    - If the row optimmization was used, it will lead to thread starvation as only one thread will do thw work.

```cpp
if (total_rows_A == 1) {
    #pragma omp parallel for
    for (size_t j = 0; j < M; j += 8) {
        if (j + 8 <= M) {
            __m256 vec_acc = _mm256_setzero_ps();
            
            // Accumulate over K in registers
            for (size_t k = 0; k < K; k++) {
                __m256 vec_A = _mm256_set1_ps(A.data[k]);
                __m256 vec_B = _mm256_loadu_ps(&B.data[k * M + j]);
                vec_acc = _mm256_fmadd_ps(vec_A, vec_B, vec_acc);
            }
            _mm256_storeu_ps(&C.data[j], vec_acc);
        } else {
            // Scalar tail fallback
            for (size_t col = j; col < M; col++) {
                float sum = 0.0f;
                for (size_t k = 0; k < K; k++) {
                    sum += A.data[k] * B.data[k * M + col];
                }
                C.data[col] = sum;
            }
        }
    }
    return C;
}
```

- `__m256 vec_acc = _mm256_setzero_ps();`
    - initializes a 256-bit YMM register to all zeros.

- The `k` loop executes in the vector × matrix fast path:
    - `A` is a 1×K row vector
    - `B` is a K×M matrix

- We are computing 8 output columns at once
    - `j` is the starting column index of the 8-column block

- Each iteration of the loop corresponds to doing:
    `vec_acc += A[k] * B[k, j : j+7]`, for one value of `k`.

- `__m256 vec_A = _mm256_set1_ps(A.data[k]);`
    - takes the scalar value `A[k]` and broadcasts it into 8 float lanes.

- `__m256 vec_B = _mm256_loadu_ps(&B.data[k * M + j]);`
    - Loads 8 consecutive floats from row k of B, starting at column j.
    - The load it retrieves is :
        - `[B[k][j], B[k][j+1], ..., B[k][j+7]]`

- `vec_acc = _mm256_fmadd_ps(vec_A, vec_B, vec_acc);`
    - Performs an FMA (Fused Multiply Add) for each lane:
        - `vec_acc = vec_acc + (vec_A * vec_B)`
        - `vec_acc[lane] += A[k] * B[k][j + lane]`, for each of the 8 columns `j`..`j+7`.
        - Peforms multiply and add in 1 instruction.

- When number of columns left are less than 8, it falls back to normal/non-intrinsic way.

---

This was for when `A` has only one row. For the cases where there are more than 1 rows, we will do the row-wise parallelism.

```cpp
// total_rows_A > 1
#pragma omp parallel for
    for (size_t i = 0; i < total_rows_A; i++) {
        for (size_t k = 0; k < K; k++) {
            float val_A = A.data[i * K + k];
            __m256 vec_A = _mm256_set1_ps(val_A);
            size_t j = 0;
            for (; j + 8 <= M; j += 8) {
                __m256 vec_B = _mm256_loadu_ps(&B.data[k * M + j]);
                __m256 vec_C = _mm256_loadu_ps(&C.data[i * M + j]);
                vec_C = _mm256_fmadd_ps(vec_A, vec_B, vec_C);
                _mm256_storeu_ps(&C.data[i * M + j], vec_C);
            }
            for (; j < M; j++) {
                C.data[i * M + j] += val_A * B.data[k * M + j];
            }
        }
    }
    return C;
```

- This is same as the last optimization discussed [above](#parallelization-openmp), but with intrinsics used.

---

## `matmul_q8_out`

To further optimize the performance, **quantization is applied**. *Weights* are converted from the standard `32-bit floats (4 byte)` to `8-bit integers (1 byte)` along with a scaling factor. This reduces the memory footprint of the model.

This is done using a struct `QTensor`

```cpp
// Struct for Quantized Tensors (Int8)
struct QTensor {
    std::vector<int8_t> data;   // The raw quantized weights (1 byte each)
    std::vector<size_t> shape;  // The dimensions of the matrix (e.g., [4096, 4096])
    float scale;                // The multiplier to convert Int8 back to Float32
};
```

- In a tensor `W`, `scale` is calculated by : 

$$ \text{scale} = \frac{\max{|W|}}{127} $$

Now lets see how the quantized multiplication works.

It accepts 3 params : `void Tensor::matmul_q8_out(const Tensor& A, const QTensor& B, Tensor& out)`

- As you can see `A` is a normal tensor but `B` is a quantized tensor.
- Before discussing about the 3rd param `Tensor& out`, some behind the scenes work should be discussed.

> **Profiling** : the process of analyzing a program's execution to understand its performance, identify bottlenecks, and optimize resource usage, such as CPU time, memory consumption, and I/O operations.

- Using `gprof` and only using 1 thread, a critical insight was revealed, the prev version (`matmul_q8` and also `matmul_2d`) was calling `Tensor::Tensor` more than a **million times** in less than 10 sec. 

This was because each layer in the model was returning a new Tensor, resulting in 1000s of heap allocations and de-allocations.

To counter this, a **Zero-Malloc** architecture is used. A struct `LayerWorkspace` is pre allocated containing all necessary intermediate buffers.

```cpp
struct LayerWorkspace {
    Tensor norm_buf;
    Tensor q, k, v;      
    Tensor concat_buf;   
    Tensor att_out;      
    Tensor ffn_hidden;   
    Tensor ffn_out;      
};
// Buffers are resized once at startup and never freed until exit.
```

- Thus, `matmul_q8_out` has **out** in it.

> `matmul_2d` didn't used this pre-allocated mechanism because the Tensors it deals with depend on the size of input. Thus allocating an initial buffer of appropriate size is difficult.

Now, `A` is float32 and `B` is int8. To perform matmul, $ B_{float} = B_{int8} \times {scale} $. And then to get final result : $ \text{Result} = A \times B_{float} $ .

But, using the associative property of mat-mul,

$$ Result = A \times (B_{int8} \times scale) = (A \times scale) \times B_{int8} $$

- If the original approach was used, then there would be 2 vector operations in each step, multiply scale with B and then FMA (fused multiplication add).
- But now there is only one vector operation inside the loop.

Now the same type of matmul operation is done like in matmul_2d.

---

This completes the core part of the engine.

> Other operations like `transpose`, `softmax` are also implemented using intrinsics.


# Working of the Model 

- It uses `MultiHeadAttention` class with KV-Cache for decoding.

- `EncoderLayer`: Comprises Self-Attention and a Feed-Forward Network (FFN) with residual connections and Layer Normalization. It processes the input source tokens.

- `DecoderLayer`: It is slightly more complex, it includes Masked Self-Attention (using the KV cache), Cross-Attention (attending to the static encoder_output), and an FFN.

- `Parallelism` : Loops are parallelized for `for` operations like embedding addition, head extraction, and tensor addition.

## Inference operation

The `Transformer` class orchestrates the generation:

- `load_from_file`: Reads a custom binary format containing hyperparameters, a magic number 2024 to verify the correct model is loaded, config, and weights. It handles the deserialization of both quantized and float tensors.

- `encode`: Processes the source sentence once. It adds position encodings to embeddings and passes them through encoder_layers.

- `decode`: Generates tokens one by one. It takes the encoder_output and the current token, runs through decoder_layers (utilizing the cache), and projects the output to vocabulary size (logits_cache) for the next token prediction.

---

> More details can be found in the [repo](https://github.com/Kush-Singh-26/Inference_Engine).

The main aim of this post was to learn about how to build an inference engine from scratch. In the process I learned many new things and more about C++.